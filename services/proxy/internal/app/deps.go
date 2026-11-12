package app

import (
	"context"
	"net"
	"net/http"

	"github.com/hilthontt/visper/services/proxy/internal/clientip"
	"github.com/hilthontt/visper/services/proxy/internal/config"
	"github.com/hilthontt/visper/services/proxy/internal/throttling"
	"github.com/redis/go-redis/v9"
)

// newThrottle builds the per-IP and global throttling levels. The most
// specific level comes first so a blocked client does not burn global quota.
func newThrottle(c config.Throttle, ips *clientip.Resolver, rdb redis.UniversalClient) *throttling.Hierarchy {
	level := func(name string, maxRequests int, key func(*http.Request) string) *throttling.Level {
		return &throttling.Level{
			Name: name,
			Key:  key,
			Throttler: throttling.New(throttling.Config{
				Name:        name,
				MaxRequests: maxRequests,
				Interval:    c.Interval,
				Spans:       c.Spans,
				Cooldown:    c.Cooldown,
			}, rdb),
		}
	}
	return throttling.NewHierarchy(
		level("ip", c.IPMaxRequests, ips.ClientIP),
		level("global", c.GlobalMaxRequests, func(*http.Request) string { return "global" }),
	)
}

// newRedisClient returns a nil interface (not a typed nil) when Redis is not
// configured.
func newRedisClient(c config.Redis) redis.UniversalClient {
	if len(c.SentinelAddrs) == 0 {
		return nil
	}
	opts := &redis.FailoverOptions{
		MasterName:       c.MasterName,
		SentinelAddrs:    c.SentinelAddrs,
		Password:         c.Password,
		SentinelPassword: c.SentinelPassword,
	}
	if c.DialLocalhost {
		// Sentinel reports internal Docker IPs (172.18.x.x) for the master.
		// Redirect them to localhost since ports are mapped to the host.
		opts.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			return (&net.Dialer{}).DialContext(ctx, "tcp4", "127.0.0.1:"+port)
		}
	}
	return redis.NewFailoverClient(opts)
}
