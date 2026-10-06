package flow

import (
	"context"
	"fmt"
)

const (
	actionResources           = "res"
	actionRefreshResources    = "rr"
	resourceNameRunes         = 200
	resourceFileRunes         = 200
	resourceFilesShown        = 5
	bytesPerKiB               = 1024
	msgNoResources            = "还没有查到已下载的资源记录哦，等 MoviePilot 找到并添加下载后再看看吧～"
	msgResourceReadFailed     = "暂时查不到资源信息，稍后再点一次试试吧～"
	msgResourceProgressFailed = "下载进度暂时查不到，资源记录仍然可以看哦～"
)

var resourceHead = []string{"站点", "下载器", "体积", "进度"}

func (e *Engine) chooseResources(ctx context.Context, sess session, p press) (Reply, bool) {
	if p.action != actionResources && p.action != actionRefreshResources {
		return Reply{}, false
	}
	if p.arg < 0 || p.arg >= len(sess.subs) {
		return Reply{Notice: msgInvalidChoice}, true
	}
	return e.resources(ctx, sess, p.arg), true
}

func (e *Engine) resources(ctx context.Context, sess session, index int) Reply {
	sub := sess.subs[index]
	resources, err := e.backend.Resources(ctx, sub.ID)
	if err != nil {
		e.log.Warn("subscription resources unavailable", "subscription", sub.ID, "err", err)
		if message, safe := UserMessage(err); safe {
			return Reply{Notice: message}
		}
		return Reply{Notice: msgResourceReadFailed}
	}
	if len(resources) == 0 {
		return Reply{Notice: msgNoResources}
	}
	cosmetic, cancel := context.WithTimeout(ctx, cosmeticTimeout)
	defer cancel()
	downloads, err := e.backend.Downloads(cosmetic)
	if err != nil {
		e.log.Warn("resource progress unavailable", "subscription", sub.ID, "err", err)
	}
	view := listView{
		heading: Heading(Plain("订阅资源 · " + taskName(truncate(sub.Title, listTitleRunes), sub.Season))),
		footer:  []Button{{Label: "刷新", Data: data(sess.id, actionRefreshResources, index)}},
	}
	if err != nil {
		view.note = msgResourceProgressFailed
	}
	for i, resource := range resources {
		text := Lines(resourceEntry(i+1, resource, downloads))
		view.entries = append(view.entries, listEntry{text: text})
	}
	return e.listPages(sess, view)
}

func resourceEntry(n int, r Resource, downloads []Download) Block {
	name := r.Name
	if name == "" {
		name = "未命名资源"
	}
	item := entry(n, Strong(truncate(name, resourceNameRunes)), truncate(EpisodeRanges(r.Episodes), listTitleRunes))
	if table := resourceTable(r, downloads); len(table.Rows) > 0 {
		item.Body = Lines(table)
	}
	for _, file := range r.Files[:min(len(r.Files), resourceFilesShown)] {
		item.Body = append(item.Body, Line(Mono(truncate(file, resourceFileRunes))))
	}
	if omitted := len(r.Files) - resourceFilesShown; omitted > 0 {
		item.Body = append(item.Body, Remark(fmt.Sprintf("还有 %d 个文件未列出，完整清单去 MoviePilot 看看吧～", omitted)))
	}
	return item
}

func resourceTable(r Resource, downloads []Download) Block {
	size, state := "", ""
	for _, d := range downloads {
		if r.DownloadID != "" && r.DownloadID == d.ID && r.Downloader == d.Downloader {
			size, state = resourceSize(d.Size), progress(d)
			break
		}
	}
	return Table(resourceHead, []Span{
		Plain(truncate(r.Site, listTitleRunes)), Plain(truncate(r.Downloader, listTitleRunes)),
		Plain(size), Plain(state),
	})
}

func resourceSize(bytes float64) string {
	if bytes <= 0 {
		return ""
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	unit := 0
	for bytes >= bytesPerKiB && unit < len(units)-1 {
		bytes /= bytesPerKiB
		unit++
	}
	return fmt.Sprintf("%.1f %s", bytes, units[unit])
}
