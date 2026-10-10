package flow

// Creation receipts identify a selected download by hash. Current queue
// and downloader observations must agree before attributing any progress.
func (r RequestRecord) ambiguousInstance(read taskReads) bool {
	instances := map[string]bool{}
	for _, download := range read.downloads {
		if download.ID != r.Download {
			continue
		}
		if download.InstanceAmbiguous {
			return true
		}
		instances[download.Downloader] = true
	}
	for _, job := range read.jobs {
		for _, file := range job.Files {
			if file.Download == r.Download {
				instances[file.Downloader] = true
			}
		}
	}
	return instances[""] || len(instances) > 1
}
