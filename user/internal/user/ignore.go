package user

// MaxIgnoredSources is how many sources of the news feed one may ignore,
// as openvk's ignoredSourcesLimit.
const MaxIgnoredSources = 50

// MaxUnignored is how many sources stop being ignored in one step, as
// openvk's newsfeed.deleteBan takes.
const MaxUnignored = 10
