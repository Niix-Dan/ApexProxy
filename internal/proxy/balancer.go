package proxy

import (
	"hash/fnv"
	"net"
	"sync/atomic"
)

func (rr *RuntimeRoute) chooseTarget(remoteAddr string) *RuntimeTarget {
	if len(rr.Targets) == 0 {
		return nil
	}

	switch rr.Config.Strategy {
	case "round-robin":
		if len(rr.FlattenedTargets) == 0 {
			return nil
		}
		idx := atomic.AddUint64(&rr.Counter, 1) - 1
		return rr.FlattenedTargets[idx%uint64(len(rr.FlattenedTargets))]

	case "ip-hash":
		ip, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			ip = remoteAddr
		}
		h := fnv.New32a()
		h.Write([]byte(ip))
		idx := h.Sum32() % uint32(len(rr.Targets))
		return rr.Targets[idx]

	case "single", "":
		fallthrough
	default:
		return rr.Targets[0]
	}
}
