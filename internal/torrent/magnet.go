package torrent

import (
	"encoding/base32"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
)

type Mag_link struct {
	Trackers  []string
	Info_hash []byte
	Name      string
}

func Parse_Mag_URL(urli string) (*Mag_link, error) {
	if urli[0:7] != "magnet:" {
		return nil, errors.New("Invalid magnet URL")
	}
	parsed_url, err := url.Parse(urli)
	if err != nil {
		return nil, errors.New("Failed to parse magnet URL")
	}
	mag := &Mag_link{}
	params := parsed_url.Query()
	for keys, vals := range params {
		switch keys {
		case "xt":
			for _, val := range vals {
				if val[0:9] != "urn:btih:" {
					continue
				}
				hash_str := val[9:]
				if len(hash_str) == 40 {
					info_hash, err := hex.DecodeString(hash_str)
					if err == nil {
						mag.Info_hash = info_hash
					}
				} else {
					hash_str = strings.ToUpper(hash_str) //some weird torrent base32 padding rule man don't question it
					info_hash, err := base32.StdEncoding.DecodeString(hash_str)
					if err == nil {
						mag.Info_hash = info_hash
					}
				}
			}
		case "dn":
			if len(vals) > 0 {
				mag.Name = vals[0]
			}
		case "tr":
			mag.Trackers = append(mag.Trackers, vals...)
		}
	}
	return mag, nil
}
