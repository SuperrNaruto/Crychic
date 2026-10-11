package moviepilot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/SuperrNaruto/Crychic/internal/notify"
)

const (
	nativePrefix        = "/api/v1/plugin/Crychic/"
	nativeDecimal       = 10
	nativeWait          = 25
	nativeBatchSize     = 32
	nativeTimeout       = 35 * time.Second
	nativeResponseLimit = 2 << 20
)

var errNativeRead = errors.New("native notification service unavailable")

func (c *Client) NativeStatus(ctx context.Context) (notify.NativeStatus, error) {
	var out notify.NativeStatus
	err := c.nativeCall(ctx, call{method: http.MethodGet, path: nativePrefix + "status"}, &out)
	return out, err
}

func (c *Client) NativeNotifications(ctx context.Context, cursor notify.NativeCursor) (notify.NativeBatch, error) {
	query := url.Values{"after": {strconv.FormatUint(cursor.Sequence, nativeDecimal)}, "consumer_id": {cursor.Consumer}, "wait": {strconv.Itoa(nativeWait)}, "limit": {strconv.Itoa(nativeBatchSize)}}
	query.Set("channel_id", cursor.Channel)
	query.Set("stream_id", cursor.Stream)
	var out notify.NativeBatch
	err := c.nativeCall(ctx, call{method: http.MethodGet, path: nativePrefix + "notifications", query: query}, &out)
	return out, err
}

func (c *Client) NativeAck(ctx context.Context, cursor notify.NativeCursor) error {
	var out struct {
		Sequence *uint64 `json:"sequence"`
	}
	if err := c.nativeCall(ctx, call{method: http.MethodPost, path: nativePrefix + "ack", body: cursor}, &out); err != nil {
		return err
	}
	if out.Sequence == nil || *out.Sequence != cursor.Sequence {
		return errNativeRead
	}
	return nil
}

func (c *Client) nativeCall(ctx context.Context, cl call, out any) error {
	ctx, cancel := context.WithTimeout(ctx, nativeTimeout)
	defer cancel()
	req, err := c.newRequest(ctx, cl)
	if err != nil {
		return errNativeRead
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return errNativeRead
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errNativeRead
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, nativeResponseLimit+1))
	if err != nil || len(raw) > nativeResponseLimit {
		return errNativeRead
	}
	return decodeNativeResponse(raw, cl, out)
}

func decodeNativeResponse(raw []byte, cl call, out any) error {
	var envelope struct {
		Success *bool           `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return errNativeRead
	}
	if envelope.Success == nil || !*envelope.Success || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return errNativeRead
	}
	if cl.method == http.MethodGet && !nativeWireHeader(envelope.Data) {
		return errNativeRead
	}
	if json.Unmarshal(envelope.Data, out) != nil {
		return errNativeRead
	}
	return nil
}

func nativeWireHeader(raw json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	for _, key := range []string{"protocol", "channel_id", "stream_id", "head", "active"} {
		value := fields[key]
		if len(value) == 0 || string(value) == "null" {
			return false
		}
	}
	return true
}
