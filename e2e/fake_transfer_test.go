package e2e

import "time"

// numberTransfer fills the history id and, unless the scenario supplies
// one, a stable completion date.
func numberTransfer(file transfer, id int) transfer {
	file.ID = id
	if file.Date == "" {
		file.Date = epoch.Add(time.Duration(id) * time.Second).Format(time.DateTime)
	}
	return file
}
