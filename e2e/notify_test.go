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
