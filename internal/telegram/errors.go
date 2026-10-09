package telegram

import (
	"context"
	"errors"
	"strings"

	"github.com/go-telegram/bot"
)

// Only explicit media rejections justify changing posters and sending again.
// Transport failures may follow a successful send; 429 is not a media error.
func rejectedPoster(err error) bool {
	if !errors.Is(err, bot.ErrorBadRequest) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "wrong file identifier/http url specified") ||
		strings.Contains(message, "failed to get http url content")
}

// Editing a known rich message is repeatable; permanent refusals and shutdown
// are not. Unknown transport results are safe to retry only for this edit.
func retryableEdit(err error) bool {
	if err == nil {
		return false
	}
	for _, permanent := range []error{
		context.Canceled, bot.ErrorBadRequest, bot.ErrorForbidden,
		bot.ErrorUnauthorized, bot.ErrorNotFound, bot.ErrorConflict,
	} {
		if errors.Is(err, permanent) {
			return false
		}
	}
	return true
}
