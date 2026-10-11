package e2e

const dunePoster = "https://image.tmdb.org/t/p/w500/6hsJknqlPceFxExOe87z5VGgNG9.jpg"

// show is a TV show as MoviePilot's transfer history names it.
type show struct{ title, year, id, image string }

var (
	breakingBad = show{"绝命毒师", "2008", "1396", "https://image.tmdb.org/t/p/w500/rqliuvX7NdknSHu5qaSDfESplQi.jpg"}
	conan       = show{"名侦探柯南", "1996", "30983", "https://image.tmdb.org/t/p/w500/7qBrY88hNwMrMb75PkBZjhFolbL.jpg"}
	mygo        = show{"迷途之子!!!!!", "2023", "224207", "https://image.tmdb.org/t/p/w500/dDknWHLYaQB76QSViNdzXOC5v64.jpg"}
)

func duneFile() transfer {
	return transfer{Title: "沙丘", Type: "电影", MediaSource: "themoviedb", MediaID: "438631", Image: dunePoster, Year: "2021"}
}

// file is a transfer record of episodes, e.g. file("S02", "E01-E03").
func (s show) file(season, episodes string) transfer {
	return transfer{Title: s.title, Type: "电视剧", MediaSource: "themoviedb", MediaID: s.id, Seasons: season, Episodes: episodes, Image: s.image, Year: s.year}
}

// The requester hears once their movie is in the library; transfers that
// predate the bot are never announced.

// In a group the notice mentions the requester, a batch of episodes becomes
// one notice, and other seasons are not announced.

// Episodes that arrive one poll apart share a notice once they settle, and
// arrivals not yet announced survive a restart. The notice words each
// download's quality once, as MoviePilot's recognizer does from one of its
// file names; a download it cannot recognize adds nothing.

// Asking for something already subscribed still earns a notice.

// Requests outlive restarts; episodes before the chosen start are ignored,
// and the requester is told when everything they asked for has arrived.

// A notice waits until the media server shows what arrived, then links to
// where it can be watched.

// A notice telling the arrivals of several watches of one season waits
// until the media server shows all of them: the subscription's E12 shows,
// the download's E01 does not yet, so neither is told before the scan.

// A shared notice says every wanted episode is in only when that holds for
// everyone it tells: Alice's subscription from E13 is complete, Bob's
// download of the season still lacks E02–E12.

// A completed movie does not enter the quiet period; disabling library
// waiting must announce it even when the media server has not scanned it.

// A requested download that stops moving is told about once: progress
// restarts the clock, and the telling survives a restart, so it is never
// repeated for the same torrent.

// A download picked for a season that is also subscribed brings files both
// requests wait for: a stuck download and its arrival are each told once,
// the arrival in words that fit the subscription as much as the download.

// A merged notice accounts for each download's own library snapshot: Alice
// requested a full season, then Bob requested its final missing episode.
