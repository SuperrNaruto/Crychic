package flow

import (
	"fmt"
	"slices"
)

type requestFailureAdvice struct {
	label string
	next  string
}

// Suggestions are ours, not upstream recovery_action text. In particular,
// downloader cleanup never proves media-server visibility.
var requestFailureLabels = map[TransferFailureStage]requestFailureAdvice{
	FailureUnknown:     {"未知阶段", "请到 MoviePilot 后台核对，不能从错误原文猜测原因。"},
	FailureSource:      {"源文件访问", "请在后台检查源文件与读取权限。"},
	FailureRecognition: {"媒体识别", "请在后台核对媒体身份和文件命名。"},
	FailurePlanning:    {"整理计划", "请在后台核对媒体身份与整理参数。"},
	FailureDestination: {"目标存储访问", "请在后台检查存储连接、目录权限和剩余空间。"},
	FailureTransfer:    {"文件转移", "请在后台检查源文件和目标存储。"},
	FailureOverwrite:   {"覆盖策略", "请在后台核对覆盖设置和已有版本。"},
	FailureCleanup:     {"下载器清理", "请在后台核对下载器任务；媒体库可见性仍需单独确认。"},
	FailureExecution:   {"任务执行", "请在后台核对任务状态；人工复核由管理员处理，机器人不会执行。"},
}

func (d requestDetail) issueText() Text {
	text := Lines(Group("异常核对"))
	if execution := d.progress.sub.Execution; execution != nil && execution.HasError {
		text = append(text, Line(Plain("订阅搜索有执行提示，具体原因请到 MoviePilot 后台查看哦～")))
	}
	if len(d.failures) == 0 {
		return append(text, Line(Plain("本次未找到可可靠关联的失败整理记录。")))
	}
	text = append(text, Line(Strong(fmt.Sprintf("关联失败记录 %d 条", len(d.failures)))))
	counts := map[TransferFailureStage]int{}
	for _, failure := range d.failures {
		counts[failure.Stage]++
	}
	for stage := FailureUnknown; stage <= FailureExecution; stage++ {
		if count := counts[stage]; count > 0 {
			advice := requestFailureLabels[stage]
			text = append(text, Line(Strong(fmt.Sprintf("%s %d 条 ", advice.label, count)), Plain(advice.next)))
		}
	}
	if slices.ContainsFunc(d.failures, func(f TransferFailure) bool { return f.AutoPaused }) {
		text = append(text, Line(Plain("部分文件的自动整理已暂停，需在 MoviePilot 后台核对。")))
	}
	if slices.ContainsFunc(d.failures, func(f TransferFailure) bool { return f.RetryExhausted }) {
		text = append(text, Line(Plain("部分文件的自动重试已用尽；这里不会重新提交整理。")))
	}
	return text
}
