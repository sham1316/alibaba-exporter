package metrics

import (
	"alibaba-exporter/config"

	"github.com/prometheus/client_golang/prometheus"
)

type Counters struct {
	cfg                *config.Config
	AvailableAmount    prometheus.Gauge
	PrepaidTraffic     prometheus.Gauge
	EcsCpu             prometheus.Gauge
	EcsRam             prometheus.Gauge
	TotalInstances     *prometheus.GaugeVec
	EcsInstances       *prometheus.GaugeVec
	PrepaidCommodities *prometheus.GaugeVec
}

func NewCounters(cfg *config.Config) *Counters {
	c := &Counters{
		cfg: cfg,
	}
	c.register()
	return c
}

func (c *Counters) register() {
	availableAmount := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "available_amount_balance",
			Help: "QueryAccountBalance availableAmount",
		})
	prometheus.MustRegister(availableAmount)
	c.AvailableAmount = availableAmount

	totalInstances := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "total_instances",
			Help: "Total instances by payment method.",
		},
		[]string{"ProductCode", "SubscriptionType", "Region", "RenewStatus", "Status", "SubStatus"},
	)
	prometheus.MustRegister(totalInstances)
	c.TotalInstances = totalInstances
	totalInstances.Reset()

	prepaidTraffic := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "prepaid_traffic",
			Help: "Total traffic available.",
		})
	prometheus.MustRegister(prepaidTraffic)
	c.PrepaidTraffic = prepaidTraffic

	prepaidCommodities := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "prepaid_commodities",
			Help: "All prepaid commodities.",
		},
		[]string{"CommodityCode"},
	)
	prometheus.MustRegister(prepaidCommodities)
	c.PrepaidCommodities = prepaidCommodities

	ecsInstances := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ecs_instances",
			Help: "Total ECS instances.",
		},
		[]string{"ProductCode", "SubscriptionType", "Region", "RenewStatus", "Status", "SubStatus"},
	)
	prometheus.MustRegister(ecsInstances)
	c.EcsInstances = ecsInstances
	ecsInstances.Reset()

	c.EcsCpu = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ecs_cpu",
			Help: "Total ECS CPU.",
		})
	prometheus.MustRegister(c.EcsCpu)

	c.EcsRam = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ecs_ram",
			Help: "Total ECS RAM.",
		})
	prometheus.MustRegister(c.EcsRam)

}
