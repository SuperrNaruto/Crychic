package notify

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

const stateDirMode = 0o755

type NativeOptions struct {
	Feed       NativeFeed
	Sender     Sender
	Path       string
	RetryEvery time.Duration
	Log        *slog.Logger
	Secrets    []string
}

type NativeConsumer struct {
	opts         NativeOptions
	cursor       NativeCursor
	lastError    string
	initialFault error
}

func NewNative(opts NativeOptions) (*NativeConsumer, error) {
	cursor, err := loadNativeCursor(opts.Path)
	return &NativeConsumer{opts: opts, cursor: cursor, initialFault: err}, nil
}

func (n *NativeConsumer) Run(ctx context.Context) {
	if n.initialFault != nil {
		n.opts.Log.Warn("native checkpoint invalid; notification reception stopped")
		<-ctx.Done()
		return
	}
	for ctx.Err() == nil {
		err := n.poll(ctx)
		if err == nil {
			n.lastError = ""
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if n.lastError != err.Error() {
			n.opts.Log.Warn("native notification unavailable", "reason", err.Error())
			n.lastError = err.Error()
		}
		timer := time.NewTimer(n.opts.RetryEvery)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (n *NativeConsumer) poll(ctx context.Context) error {
	status, err := n.opts.Feed.NativeStatus(ctx)
	if err != nil {
		return err
	}
	if !validNativeStatus(status) || !status.Active {
		return errors.New("native channel inactive or incompatible")
	}
	if err := n.baseline(status); err != nil {
		return err
	}
	if !n.sameSource(status) || status.Head < n.cursor.Sequence {
		return errors.New("native channel identity or progress changed")
	}
	if err := n.opts.Feed.NativeAck(ctx, n.cursor); err != nil {
		return err
	}
	batch, err := n.opts.Feed.NativeNotifications(ctx, n.cursor)
	if err != nil {
		return err
	}
	if err := n.validate(batch); err != nil {
		return err
	}
	return n.consume(ctx, batch.Messages)
}

func (n *NativeConsumer) baseline(status NativeStatus) error {
	if n.cursor.Consumer != "" {
		return nil
	}
	cursor, err := newNativeCursor(status)
	if err != nil {
		return err
	}
	return n.persist(cursor)
}

func validNativeStatus(status NativeStatus) bool {
	return status.Protocol == NativeProtocol && status.Channel != "" && status.Stream != ""
}

func (n *NativeConsumer) sameSource(status NativeStatus) bool {
	return n.cursor.Channel == status.Channel && n.cursor.Stream == status.Stream
}

func (n *NativeConsumer) validate(batch NativeBatch) error {
	if !n.validBatchSource(batch) {
		return errors.New("native response is incomplete")
	}
	sequence := n.cursor.Sequence
	ids := map[string]bool{}
	for _, message := range batch.Messages {
		if !validNativeSequence(message, sequence, batch.Head) || ids[message.ID] {
			return errors.New("native notification sequence has a gap")
		}
		if _, err := n.notice(message); err != nil {
			return err
		}
		sequence, ids[message.ID] = message.Sequence, true
	}
	if sequence < batch.Head && len(batch.Messages) == 0 {
		return errors.New("native notification range is missing")
	}
	return nil
}

func (n *NativeConsumer) validBatchSource(batch NativeBatch) bool {
	return validNativeStatus(batch.NativeStatus) && batch.Active && n.sameSource(batch.NativeStatus) && batch.Messages != nil
}

func validNativeSequence(message NativeMessage, after, head uint64) bool {
	return message.Sequence == after+1 && message.Sequence <= head && message.ID != ""
}

func (n *NativeConsumer) consume(ctx context.Context, messages []NativeMessage) error {
	for _, message := range messages {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		notice, err := n.notice(message)
		if err != nil {
			return err
		}
		cursor := n.cursor
		cursor.Sequence = message.Sequence
		if err := n.persist(cursor); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := n.opts.Sender.Notify(ctx, notice); err != nil && ctx.Err() == nil {
			n.opts.Log.Warn("native notification delivery failed; will not replay")
		}
		if err := n.opts.Feed.NativeAck(ctx, n.cursor); err != nil {
			return err
		}
	}
	return nil
}

func (n *NativeConsumer) persist(cursor NativeCursor) error {
	if err := saveNativeCursor(n.opts.Path, cursor); err != nil {
		return err
	}
	n.cursor = cursor
	return nil
}
