package torrent

import "strings"

func (t *Torrent) GetAnnounceURL() string {
	if t.Announce != "" {
		return t.Announce
	}

	for _, tier := range t.AnnounceList {
		for _, url := range tier {
			if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
				return url
			}
		}
	}
	return ""
}
