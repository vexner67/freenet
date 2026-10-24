package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// rankFeedTemplate ranks a feed query (%[1]s), whose reader is %[2]s, by
// post.Interest as of $8: the likes and comments the reader gave each
// source in post.AffinityPeriod (affinity; their own posts count in
// full), and every post of the week scored by the formula of
// Interest.Score. $9 and $10 are the score and the ID the previous page
// ended with, NULL for the first one.
const rankFeedTemplate = `
	WITH affinity AS (
		SELECT
			CASE WHEN p.from_wall THEN p.wall_id ELSE p.author_id END AS source,
			count(*) AS n
		FROM (
			SELECT post_id FROM post_likes
			WHERE user_id = %[2]s AND created_at > $8::timestamptz - make_interval(days => %[3]d)
			UNION ALL
			SELECT post_id FROM comments
			WHERE author_id = %[2]s AND created_at > $8::timestamptz - make_interval(days => %[3]d)
		) AS touched
		JOIN posts p ON p.id = touched.post_id
		GROUP BY 1
	),
	feed AS (%[1]s),
	scored AS (
		SELECT
			feed.*,
			(
				1 + feed.like_count + 2 * feed.comment_count + 3 * feed.repost_count
				+ 2 * least(
					CASE
						WHEN source.id = %[2]s THEN %[4]d
						ELSE coalesce(affinity.n, 0)
					END,
					%[4]d
				)
				+ CASE WHEN cardinality(feed.photo_ids) > 0 THEN 1 ELSE 0 END
			)::float8
			/ power((greatest(extract(epoch FROM $8::timestamptz - feed.created_at), 0) / 3600 + 2)::float8, 1.5) AS score
		FROM feed
		CROSS JOIN LATERAL (
			SELECT CASE WHEN feed.from_wall THEN feed.wall_id ELSE feed.author_id END AS id
		) AS source
		LEFT JOIN affinity ON affinity.source = source.id
	)
	SELECT
		id,
		wall_id,
		author_id,
		text,
		photo_ids,
		created_at,
		repost_of,
		like_count,
		comment_count,
		repost_count,
		edited_at,
		from_wall,
		signed,
		suggested,
		shared_photo_id,
		archived,
		comments_closed,
		nsfw,
		price,
		polled,
		document_ids,
		link,
		pinned,
		score
	FROM scored
	WHERE $9::float8 IS NULL OR (score, id) < ($9::float8, $10::uuid)
	ORDER BY score DESC, id DESC
`

// rankFeedQuery and rankGlobalFeedQuery are the news of listFeedQuery and
// listGlobalFeedQuery from the start of the week on ($2 and $1), unsorted
// and unbounded, ranked; the limit is the one of the plain queries.
var (
	rankFeedQuery       = rankQuery(listFeedQuery, "$1", "AND id < $2", "LIMIT $3")
	rankGlobalFeedQuery = rankQuery(listGlobalFeedQuery, "$7", "WHERE id < $1", "LIMIT $2")
)

// rankQuery turns a plain feed query into a ranked one: its bound below
// becomes a bound above, and its order and limit move outside.
func rankQuery(feed, reader, bound, limit string) string {
	inner := strings.Replace(feed, bound, strings.Replace(bound, "<", ">=", 1), 1)
	cut := strings.Index(inner, "ORDER BY id DESC")
	if cut < 0 || !strings.Contains(feed, bound) || !strings.Contains(feed, limit) {
		panic("postgres: a feed query changed its shape")
	}

	return fmt.Sprintf(rankFeedTemplate, inner[:cut], reader, int(post.AffinityPeriod/(24*time.Hour)), post.MaxAffinity) + limit
}

func (r *PostRepository) RankFeed(ctx context.Context, sources service.FeedSources, at time.Time, after *service.RankMark, limit int) ([]service.Ranked, error) {
	start := post.WindowStart(at.Add(-post.RankWindow))

	var score, id any
	if after != nil {
		score, id = after.Score, after.ID
	}

	var (
		rows pgx.Rows
		err  error
	)
	if sources.Everyone {
		rows, err = r.conn(ctx).Query(ctx, rankGlobalFeedQuery,
			start, limit, sources.HideNSFW, ids(sources.Hidden), sources.OwnOnly, ids(sources.Ignored), reader(sources.Reader),
			at, score, id,
		)
	} else {
		rows, err = r.conn(ctx).Query(ctx, rankFeedQuery,
			sources.Reader, start, limit, sources.HideNSFW, ids(sources.Hidden), sources.OwnOnly, ids(sources.Ignored),
			at, score, id,
		)
	}
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var ranked []service.Ranked
	for rows.Next() {
		var s float64
		p, err := scanPost(scored{rows, &s})
		if err != nil {
			return nil, errs.Wrap("scanPost", err)
		}
		ranked = append(ranked, service.Ranked{Post: p, Score: s})
	}

	return ranked, errs.Wrap("rows.Err", rows.Err())
}

// scored reads a post with its score in the last column.
type scored struct {
	pgx.Rows
	score *float64
}

func (s scored) Scan(dest ...any) error {
	return s.Rows.Scan(append(dest, s.score)...)
}
