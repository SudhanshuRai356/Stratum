package torrent

import "strings"

func (t *Torrent) GetAllAnnounceURLs() []string {
	var urls []string

	if t.Announce != "" {
		urls = append(urls, t.Announce)
	}

	for _, tier := range t.AnnounceList {
		for _, u := range tier {
			if strings.HasPrefix(u, "http://") ||
				strings.HasPrefix(u, "https://") ||
				strings.HasPrefix(u, "udp://") {
				urls = append(urls, u)
			}
		}
	}

	return urls
}
