package notify

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const nativeIdentityBytes = 16

func loadNativeCursor(path string) (NativeCursor, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return NativeCursor{}, nil
	}
	if err != nil {
		return NativeCursor{}, errors.New("native checkpoint read failed")
	}
	var cursor NativeCursor
	if json.Unmarshal(raw, &cursor) != nil || cursor.Consumer == "" || cursor.Channel == "" || cursor.Stream == "" {
		return NativeCursor{}, errors.New("native checkpoint is invalid")
	}
	return cursor, nil
}

func saveNativeCursor(path string, cursor NativeCursor) error {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return errors.New("native checkpoint encode failed")
	}
	if os.MkdirAll(filepath.Dir(path), stateDirMode) != nil {
		return errors.New("native checkpoint directory unavailable")
	}
	if os.WriteFile(path+".tmp", raw, stateFileMode) != nil {
		return errors.New("native checkpoint write failed")
	}
	if os.Rename(path+".tmp", path) != nil {
		return errors.New("native checkpoint commit failed")
	}
	return nil
}

func newNativeCursor(status NativeStatus) (NativeCursor, error) {
	var identity [nativeIdentityBytes]byte
	if _, err := rand.Read(identity[:]); err != nil {
		return NativeCursor{}, errors.New("native identity unavailable")
	}
	return NativeCursor{Consumer: hex.EncodeToString(identity[:]), Channel: status.Channel, Stream: status.Stream, Sequence: status.Head}, nil
}
