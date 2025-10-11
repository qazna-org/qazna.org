package obs

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	buildInfoOnce sync.Once

	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "build_info",
			Help: "Qazna API build information.",
		},
		[]string{"version", "commit"},
	)
)

func InitBuildInfo(version, commit string) {
	buildInfoOnce.Do(func() {
		prometheus.MustRegister(buildInfo)
	})

	buildInfo.WithLabelValues(version, commit).Set(1)
}
