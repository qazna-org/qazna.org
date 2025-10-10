package obs

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	buildInfoOnce sync.Once

	// buildInfo — gauge со статич. значением 1 и метками версии/коммита.
	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "build_info",
			Help: "Qazna API build information.",
		},
		[]string{"version", "commit"},
	)
)

// InitBuildInfo регистрирует метрику build_info (однократно) и устанавливает значение.
func InitBuildInfo(version, commit string) {
	buildInfoOnce.Do(func() {
		// Регистрируем в стандартном реестре (без кастомной переменной reg)
		prometheus.MustRegister(buildInfo)
	})

	// выставляем build_info{version="...", commit="..."} 1
	buildInfo.WithLabelValues(version, commit).Set(1)
}
