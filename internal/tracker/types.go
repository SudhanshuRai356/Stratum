package tracker

import "sync/atomic"

type Peer struct {
	IP   string
	Port uint16
}

var lastSeeders atomic.Int64
var lastLeechers atomic.Int64

func updateSwarmStats(seeders, leechers int) {
	if seeders >= 0 {
		lastSeeders.Store(int64(seeders))
	}
	if leechers >= 0 {
		lastLeechers.Store(int64(leechers))
	}
}

func LastSwarmStats() (seeders int, leechers int) {
	return int(lastSeeders.Load()), int(lastLeechers.Load())
}
