package notify

import (
	"errors"
	"io/fs"
	"os"
	"slices"
)

// Archive the original bytes before changing legacy notification-only fields.
// Neither old pending notices nor historical deliveries seed new completions.
func migrateRequestState(path string, st state) (state, error) {
	if st.NativeRequests {
		return st, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return state{}, errors.New("request migration read failed")
	}
	if err == nil {
		if err := archiveRequestState(path, raw); err != nil {
			return state{}, err
		}
	}
	st.NativeRequests = true
	st.Watches = slices.Clone(st.Watches)
	for i, w := range st.Watches {
		w.Pending, w.Image, w.Releases, w.Stalled, w.ArrivedAt = nil, "", nil, nil, nil
		st.Watches[i] = w
	}
	if err == nil && saveState(path, st) != nil {
		return state{}, errors.New("request migration commit failed")
	}
	return st, nil
}

func archiveRequestState(path string, raw []byte) error {
	archive := path + ".pre-native"
	if _, err := os.Stat(archive); err == nil {
		return nil
	}
	if os.WriteFile(archive+".tmp", raw, stateFileMode) != nil {
		return errors.New("request migration archive failed")
	}
	if os.Rename(archive+".tmp", archive) != nil {
		return errors.New("request migration archive commit failed")
	}
	return nil
}
