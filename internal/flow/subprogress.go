package flow

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

const (
	maxSubEpisodes = 100
	subTaskRunes   = 60
)

type subTaskProgress struct {
	episodes  map[int][]string
	ambiguous bool
}

type subProgressGroup struct {
	episodes []int
	library  string
	tasks    string
}

// progressRows groups consecutive episodes with identical observations.
// Library and task columns remain independent, including during upgrades.
func (d subscriptionDetail) progressRows() ([][]Span, int, bool) {
	tasks := d.taskProgress()
	episodes, omitted := d.shownEpisodes(tasks)
	var groups []subProgressGroup
	for _, ep := range episodes {
		library, activity := d.libraryState(ep), d.taskState(tasks.episodes[ep])
		if len(groups) > 0 && groups[len(groups)-1].continues(ep, library, activity) {
			last := &groups[len(groups)-1]
			last.episodes = append(last.episodes, ep)
			continue
		}
		groups = append(groups, subProgressGroup{episodes: []int{ep}, library: library, tasks: activity})
	}
	var rows [][]Span
	for _, group := range groups {
		label := "电影"
		if d.sub.Kind == TV {
			label = EpisodeRanges(group.episodes)
		}
		rows = append(rows, []Span{Plain(label), Plain(group.library), Plain(group.tasks)})
	}
	return rows, omitted, tasks.ambiguous
}

func (g subProgressGroup) continues(ep int, library, tasks string) bool {
	return ep == g.episodes[len(g.episodes)-1]+1 && g.library == library && g.tasks == tasks
}

func (d subscriptionDetail) shownEpisodes(tasks subTaskProgress) ([]int, int) {
	if d.sub.Kind == Movie {
		return []int{0}, 0
	}
	start := max(d.sub.StartEpisode, 1)
	if d.sub.Total > 0 {
		count := max(d.sub.Total-start+1, 0)
		shown := min(count, maxSubEpisodes)
		episodes := make([]int, shown)
		for i := range episodes {
			episodes[i] = start + i
		}
		return episodes, count - shown
	}
	var episodes []int
	if d.sub.Season != nil {
		episodes = slices.Clone(d.library.Episodes[*d.sub.Season])
	}
	for ep := range tasks.episodes {
		episodes = append(episodes, ep)
	}
	episodes = slices.DeleteFunc(episodes, func(ep int) bool { return ep < start })
	slices.Sort(episodes)
	episodes = slices.Compact(episodes)
	shown := min(len(episodes), maxSubEpisodes)
	return episodes[:shown], len(episodes) - shown
}

func (d subscriptionDetail) libraryState(ep int) string {
	if d.libraryErr != nil {
		return "未知"
	}
	if d.sub.Kind == Movie && d.library.Movie {
		return "已入库"
	}
	if d.sub.Season != nil && slices.Contains(d.library.Episodes[*d.sub.Season], ep) {
		return "已入库"
	}
	return "未入库"
}

func (d subscriptionDetail) taskState(states []string) string {
	if len(states) > 0 {
		return truncate(strings.Join(states, " · "), subTaskRunes)
	}
	if d.downloadsErr != nil || d.transfersErr != nil {
		return "未知"
	}
	return "暂无任务记录"
}

func (d subscriptionDetail) taskProgress() subTaskProgress {
	progress := subTaskProgress{episodes: map[int][]string{}}
	for _, download := range d.downloads {
		d.addDownload(&progress, download)
	}
	for _, job := range d.transfers {
		d.addTransfer(&progress, job)
	}
	return progress
}

func (d subscriptionDetail) sameMedia(kind Kind, source, id string) bool {
	return kind == d.sub.Kind && source == d.sub.Source && id == d.sub.MediaID
}

func (d subscriptionDetail) sameSeason(season *int) bool {
	if d.sub.Kind == Movie {
		return season == nil || *season == 0
	}
	return season != nil && d.sub.Season != nil && *season == *d.sub.Season
}

func (d subscriptionDetail) addDownload(p *subTaskProgress, download Download) {
	if !d.sameMedia(download.Kind, download.Source, download.MediaID) {
		return
	}
	if !d.sameSeason(download.Season) {
		p.ambiguous = p.ambiguous || download.Season == nil
		return
	}
	episodes := download.Episodes
	if d.sub.Kind == Movie {
		episodes = []int{0}
	}
	if len(episodes) == 0 {
		p.ambiguous = true
		return
	}
	label := fmt.Sprintf("下载中 %.0f%%", download.Progress)
	if download.Paused {
		label = fmt.Sprintf("已暂停 %.0f%%", download.Progress)
	}
	p.add(episodes, label)
}

var subTransferText = map[FileState]string{
	FileWaiting: "等待整理", FileRunning: "整理中", FileDone: "整理完成", FileFailed: "整理失败",
}

func (d subscriptionDetail) addTransfer(p *subTaskProgress, job TransferJob) {
	if !d.sameMedia(job.Kind, job.Source, job.MediaID) {
		return
	}
	if !d.sameSeason(job.Season) {
		p.ambiguous = p.ambiguous || job.Season == nil
		return
	}
	for _, file := range job.Files {
		if d.sub.Kind == TV && file.Episode <= 0 {
			p.ambiguous = true
			continue
		}
		ep := file.Episode
		if d.sub.Kind == Movie {
			ep = 0
		}
		p.add([]int{ep}, cmp.Or(subTransferText[file.State], "整理状态未知"))
	}
}

func (p *subTaskProgress) add(episodes []int, label string) {
	for _, ep := range episodes {
		if !slices.Contains(p.episodes[ep], label) {
			p.episodes[ep] = append(p.episodes[ep], label)
		}
	}
}
