package grpc

import (
	"github.com/vexner67/freenet/pkg/mtls"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// InternalMethods are for other services only, by mTLS identity
// (see mtls.RestrictMethods).
func InternalMethods() map[string][]string {
	return map[string][]string{
		postv1.PostService_PurgeUser_FullMethodName:            {mtls.User},
		postv1.PostService_HoldUserReactions_FullMethodName:    {mtls.User},
		postv1.PostService_ReleaseUserReactions_FullMethodName: {mtls.User},
		postv1.PostService_FollowSource_FullMethodName:         {mtls.User, mtls.Club},
		postv1.PostService_UnfollowSource_FullMethodName:       {mtls.User, mtls.Club},
		postv1.PostService_SyncSources_FullMethodName:          {mtls.User, mtls.Club},
		postv1.PostService_CloseWall_FullMethodName:            {mtls.User, mtls.Club},
		postv1.PostService_OpenWall_FullMethodName:             {mtls.User, mtls.Club},
	}
}

// PublicMethods can be called without an access token; every other RPC
// requires one (see jwtauth.UnaryServerInterceptor). The internal methods
// carry no user token: other services call them, checked by mTLS.
func PublicMethods() []string {
	return []string{
		grpc_health_v1.Health_Check_FullMethodName,
		postv1.PostService_GetPost_FullMethodName,
		postv1.PostService_GetPosts_FullMethodName,
		postv1.PostService_CountPosts_FullMethodName,
		postv1.PostService_ListPosts_FullMethodName,
		postv1.PostService_ListFeed_FullMethodName,
		postv1.PostService_ListComments_FullMethodName,
		postv1.PostService_ListPostLikers_FullMethodName,
		postv1.PostService_ListReposts_FullMethodName,
		postv1.PostService_ListCommentLikers_FullMethodName,
		postv1.PostService_GetPolls_FullMethodName,
		postv1.PostService_ListPollVoters_FullMethodName,
		postv1.PostService_GetComments_FullMethodName,
		postv1.PostService_ListLastComments_FullMethodName,
		postv1.PostService_GetTopic_FullMethodName,
		postv1.PostService_ListTopics_FullMethodName,
		postv1.PostService_ListTopicMessages_FullMethodName,
		postv1.PostService_GetTopicMessage_FullMethodName,
		postv1.PostService_GetTopicMessages_FullMethodName,
		postv1.PostService_ListTopicMessageLikers_FullMethodName,
		postv1.PostService_GetAlbum_FullMethodName,
		postv1.PostService_ListAlbums_FullMethodName,
		postv1.PostService_ListAlbumPhotos_FullMethodName,
		postv1.PostService_GetAlbumPhoto_FullMethodName,
		postv1.PostService_ListAlbumPhotoLikers_FullMethodName,
		postv1.PostService_ListPhotoComments_FullMethodName,
		postv1.PostService_GetPhotoComment_FullMethodName,
		postv1.PostService_GetPhotoComments_FullMethodName,
		postv1.PostService_ListPhotoCommentLikers_FullMethodName,
		postv1.PostService_GetNote_FullMethodName,
		postv1.PostService_ListNotes_FullMethodName,
		postv1.PostService_ListNoteComments_FullMethodName,
		postv1.PostService_GetNoteComment_FullMethodName,
		postv1.PostService_GetNoteComments_FullMethodName,
		postv1.PostService_PurgeUser_FullMethodName,
		postv1.PostService_HoldUserReactions_FullMethodName,
		postv1.PostService_ReleaseUserReactions_FullMethodName,
		postv1.PostService_FollowSource_FullMethodName,
		postv1.PostService_UnfollowSource_FullMethodName,
		postv1.PostService_SyncSources_FullMethodName,
		postv1.PostService_CloseWall_FullMethodName,
		postv1.PostService_OpenWall_FullMethodName,
	}
}
