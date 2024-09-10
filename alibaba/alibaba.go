package alibaba

import (
	"alibaba-exporter/config"
	"alibaba-exporter/metrics"
	"context"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/bssopenapi"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"strconv"
	"strings"
	"time"
)

type Alibaba interface {
	GetAccountBalance(ctx context.Context) float64
	MainLoop(ctx context.Context)
}

type alibaba struct {
	cfg      *config.Config
	client   *bssopenapi.Client
	counters *metrics.Counters
}

func NewAlibaba(cfg *config.Config, counters *metrics.Counters) (Alibaba, error) {
	sdkConfig := sdk.NewConfig()
	sdkConfig.Scheme = "https"
	cred := credentials.NewAccessKeyCredential(cfg.AlibabaConf.AccessKeyId, string(cfg.AlibabaConf.AccessKeySecret))
	client, _err := bssopenapi.NewClientWithOptions(cfg.AlibabaConf.RegionId, sdkConfig, cred)

	if _err != nil {
		return nil, _err
	}

	alibaba := &alibaba{
		cfg:      cfg,
		client:   client,
		counters: counters,
	}
	return alibaba, nil
}

func (a *alibaba) GetAccountBalance(ctx context.Context) (availableAmount float64) {
	availableAmount = 0
	var err error
	if _result, _err := a.client.QueryAccountBalance(bssopenapi.CreateQueryAccountBalanceRequest()); _err != nil {
		zap.S().Warn(_err)
	} else {
		if availableAmount, err = strconv.ParseFloat(strings.ReplaceAll(_result.Data.AvailableAmount, ",", ""), 64); err != nil {
			zap.S().Warn(err)
		}
	}
	zap.S().Debugf("GetAccountBalance: %0.2f", availableAmount)
	return availableAmount
}
func (a *alibaba) GetTotalInstanceType(ctx context.Context, subscriptionType string) (totalInstances float64) {
	request := bssopenapi.CreateQueryAvailableInstancesRequest()
	request.SubscriptionType = subscriptionType
	if _result, _err := a.client.QueryAvailableInstances(request); _err != nil {
		zap.S().Warn(_err)
	} else {
		zap.S().Debugf("GetInstanceType(%s): %d", subscriptionType, _result.Data.TotalCount)
		totalInstances = float64(_result.Data.TotalCount)
	}
	return
}

func (a *alibaba) GetAllAvailableInstance(ctx context.Context) []bssopenapi.Instance {
	var instanceList []bssopenapi.Instance

	request := bssopenapi.CreateQueryAvailableInstancesRequest()

	if _result, _err := a.client.QueryAvailableInstances(request); _err != nil {
		zap.S().Warn(_err)
	} else {
		total := _result.Data.TotalCount
		pageSize := _result.Data.PageSize
		instanceList = append(instanceList, _result.Data.InstanceList...)
		for i := 2; i <= 1+total/pageSize; i++ {
			request.PageNum = requests.NewInteger(i)
			if _result, _err := a.client.QueryAvailableInstances(request); _err != nil {
				zap.S().Warn(_err)
			} else {
				instanceList = append(instanceList, _result.Data.InstanceList...)
				zap.S().Debugf("instanceList size = %d", len(instanceList))
			}
		}
	}
	return instanceList
}

func (a *alibaba) GetDiskList(ctx context.Context) {

}
func makeVector(vector *prometheus.GaugeVec, instanceList []bssopenapi.Instance) {
	vector.Reset()
	for _, inst := range instanceList {
		vector.WithLabelValues(inst.ProductCode, inst.SubscriptionType, inst.Region, inst.RenewStatus, inst.Status, inst.SubStatus).Add(1)
	}
}

func (a *alibaba) MainLoop(ctx context.Context) {
	a.getAlibabaMetrics(ctx)
	ticker := time.NewTicker(time.Second * time.Duration(a.cfg.Interval))
	for {
		select {
		case <-ctx.Done():
			zap.S().Info("finish main context")
			return
		case _ = <-ticker.C:
			a.getAlibabaMetrics(ctx)
		}
	}
}
func (a *alibaba) getAlibabaMetrics(ctx context.Context) {
	start := time.Now()
	zap.S().Infof("%s start getAlibaba", start)
	a.counters.AvailableAmount.Set(a.GetAccountBalance(ctx))
	makeVector(a.counters.TotalInstances, a.GetAllAvailableInstance(ctx))
	a.GetDiskList(ctx)
	elapsed := time.Since(start)
	zap.S().Infof("%s finish getAlibaba (took %s)", time.Now(), elapsed)
}
