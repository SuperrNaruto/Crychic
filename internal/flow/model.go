// Package flow is the platform-agnostic request conversation: search, pick,
// (pick a season), confirm, subscribe. Chat platforms only render its Replies
// and feed back the opaque button data.
package flow

import (
	"context"
	"errors"
	"time"
)

// Kind is the media category a subscription is made for.
type Kind int

const (
	Movie Kind = iota + 1
	TV
)

func (k Kind) String() string {
	if k == TV {
		return "电视剧"
	}
	return "电影"
}

// Media identifies one search result by its backend identity, with the
// metadata search already returns.
type Media struct {
	Source        string
	ID            string
	Title         string
	OriginalTitle string
	Year          string
	Kind          Kind
	Rating        float64
	PosterURL     string
	Link          string // the media's page on its metadata site
	Overview      string
	Released      string // first release or air date, YYYY-MM-DD, when known
	Weekday       int    // calendar picks: 1 Monday … 7 Sunday it airs on
	CalendarID    string // calendar picks: the calendar's own id, kept by a TMDB twin
}

// Chart is a list of picks to discover media from.
type Chart int

const (
	Trending Chart = iota
	HotMovies
	HotShows
	InTheaters
	NewAnime
)

// Details is metadata only fetched once a result is picked.
type Details struct {
	Genres   []string
	Runtime  int // minutes, movies only
	Seasons  int // TV only
	Episodes int // TV only
	Cast     []string
	Next     Episode // next episode to air; zero when unknown or ended
	Overview string
}

// Episode locates one episode of a show.
type Episode struct {
	Season int
	Number int
}

// Season is one subscribable season of a TV show.
type Season struct {
	Number       int
	Name         string
	EpisodeCount int
}

// Target is what gets subscribed: a movie, or one season of a show,
// optionally skipping the episodes before StartEpisode (0 means from the
// season's beginning).
type Target struct {
	Media        Media
	Season       *int
	StartEpisode int
}

// Library is what the media server already holds of one title.
type Library struct {
	Movie    bool          // the movie is in the library
	Episodes map[int][]int // season number → episodes in the library
}

// Download is an unfinished task in the backend's downloader.
type Download struct {
	ID       string // stable while the task exists
	Source   string
	MediaID  string
	Kind     Kind // zero when the backend did not identify its media type
	Title    string
	Image    string
	Season   *int    // nil for movies or when unknown
	Episodes []int   // empty when unknown
	Size     float64 // bytes being downloaded, 0 when unknown
	Progress float64 // percent
	Paused   bool
	Speed    string // download speed as the backend words it
	Left     string // remaining time as the backend words it, "" when unknown
}

// FileState is where one file of a transfer job stands.
type FileState int

const (
	FileWaiting FileState = iota
	FileRunning
	FileDone
	FileFailed
)

// TransferFile is one file being moved into the library.
type TransferFile struct {
	Episode int // 0 when not an episode
	State   FileState
}

// TransferJob is a batch of files of one title (and season) the backend is
// moving into the library.
type TransferJob struct {
	ID      string // stable while the job exists
	Source  string
	MediaID string
	Kind    Kind // zero when the backend did not identify its media type
	Title   string
	Image   string
	Season  *int
	Files   []TransferFile
}

// LibraryItem is a recent addition to a media server.
type LibraryItem struct {
	Title  string
	Year   string
	Kind   string // as the backend words it, e.g. 电视剧
	Server string // the media server's name, e.g. Emby
	Link   string // where to watch it, "" when unknown
}

// Subscription is one of the backend's subscriptions.
type Subscription struct {
	ID           int
	Source       string
	MediaID      string
	Title        string
	Year         string
	Kind         Kind
	Season       *int
	State        string // backend state code, e.g. "R" while subscribed
	Lack         int    // episodes still missing from the subscription, not library
	Total        int    // episodes in the season, 0 when unknown
	StartEpisode int
	Poster       string
	Quality      string
	Resolution   string
	Effect       string
	FilterGroups []string
	BestVersion  bool
	LastSearch   time.Time
	Execution    *SubscriptionExecution
}

// SubscriptionExecution describes the last search, not download completion.
// Backend errors may contain server paths or credentials; only their presence
// crosses this boundary, never their original text.
type SubscriptionExecution struct {
	State    string
	NextRun  time.Time
	HasError bool
}

// PastSubscription is a finished or cancelled subscription in the backend's
// history. Record is the backend's own copy of it, handed back unchanged to
// Resubscribe so the new subscription keeps the old one's settings.
type PastSubscription struct {
	ID           int
	Media        Media
	Season       *int
	StartEpisode int
	Date         string // when it entered the history, as the backend words it
	Record       []byte
}

// Backend is the media server the bot subscribes through.
type Backend interface {
	Search(ctx context.Context, term string) ([]Media, error)
	Details(ctx context.Context, media Media) (Details, error)
	Seasons(ctx context.Context, media Media) ([]Season, error)
	Library(ctx context.Context, media Media) (Library, error)
	Downloads(ctx context.Context) ([]Download, error)
	Transfers(ctx context.Context) ([]TransferJob, error)
	// Discover lists a chart's picks; they may come from other metadata
	// sources than search does.
	Discover(ctx context.Context, chart Chart) ([]Media, error)
	// Related lists media recommended to fans of media; empty when unknown.
	Related(ctx context.Context, media Media) ([]Media, error)
	// Series lists the movies of the series movie media belongs to, in
	// release order; empty when it belongs to none.
	Series(ctx context.Context, media Media) ([]Media, error)
	Subscriptions(ctx context.Context) ([]Subscription, error)
	// SubscriptionHistory lists at most count past subscriptions of kind,
	// newest first.
	SubscriptionHistory(ctx context.Context, kind Kind, count int) ([]PastSubscription, error)
	// Resubscribe subscribes past again with its own settings and returns
	// the new subscription's id.
	Resubscribe(ctx context.Context, past PastSubscription) (int, error)
	// Latest lists the newest additions to the media servers.
	Latest(ctx context.Context) ([]LibraryItem, error)
	// Unsubscribe deletes a subscription; deleting one already gone is fine.
	Unsubscribe(ctx context.Context, id int) error
	// FindSubscription returns the id of an existing subscription, 0 if none.
	FindSubscription(ctx context.Context, target Target) (int, error)
	// Subscribe creates a subscription and returns its id.
	Subscribe(ctx context.Context, target Target) (int, error)
}

// Calendar lists the shows airing this season, each with its weekday.
type Calendar interface {
	Calendar(ctx context.Context) ([]Media, error)
	// Summary is a calendar pick's synopsis, "" when it has none.
	Summary(ctx context.Context, id string) (string, error)
}

// Request is a subscription someone asked for, to be told when it arrives.
type Request struct {
	SubscriptionID int
	Target         Target
	SeasonEpisodes int   // episodes in the requested season, 0 if unknown or a movie
	Held           []int // episodes of the season already in the library
	Requester      Actor
}

// Watcher remembers requests so requesters hear about arrivals.
type Watcher interface {
	Watch(ctx context.Context, req Request) error
	// Requested lists the subscriptions userID asked for.
	Requested(userID int64) []int
	// Forget drops a subscription's requests, e.g. once it is cancelled.
	Forget(ctx context.Context, subscriptionID int) error
}

// Actor is the chat user driving a conversation, in platform terms.
// Address is where the platform can reach them later (opaque to flow).
type Actor struct {
	UserID  int64
	Name    string
	Address string
}

// Notice is a message sent to someone outside any conversation, e.g. an
// arrival; Image is a poster URL to show with it. A To with UserID zero is
// an identity-free broadcast to Address, not a request to mention a user.
type Notice struct {
	To    Actor
	Text  Text
	Image string
}

// Button is one choice offered to the user; Data comes back to Choose.
type Button struct {
	Label string
	Data  string
}

// Banner names a bundled image for the home menu; empty means none.
type Banner string

const BannerHome Banner = "home"

// Banners lists every Banner; a platform checks it has an image for each.
var Banners = []Banner{BannerHome}

// Reply is what the platform shows after an action. Image is a poster URL
// to show with the text; Gallery, used only without Image, holds the posters
// of a listed page in list order. A Reply with Notice set leaves the conversation
// message untouched and only flashes the notice. A Reply with Input set asks
// the user to type an answer, which the platform hands to Engine.Answer
// together with Input.
//
// A Reply with Follow set is live: after a short wait the platform passes
// Follow to Choose and shows the new Reply in its place (only if it changed),
// until a Reply comes back without Follow or with a Notice.
type Reply struct {
	Text    Text
	Image   string
	Gallery []string
	Banner  Banner // show the named built-in banner above the home menu
	Buttons [][]Button
	Notice  string
	Input   string
	Follow  string
}

// UserError carries a message that is safe and useful to show the user.
type UserError struct{ Message string }

func (e *UserError) Error() string { return e.Message }

// UserMessage extracts the user-facing text of err, if it has one.
func UserMessage(err error) (string, bool) {
	var ue *UserError
	if errors.As(err, &ue) {
		return ue.Message, true
	}
	return "", false
}
