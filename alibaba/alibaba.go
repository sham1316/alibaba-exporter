package alibaba

import (
	"alibaba-exporter/config"
	"alibaba-exporter/metrics"
	"context"
	bssopenapi20171214 "github.com/alibabacloud-go/bssopenapi-20171214/v6/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
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
	GetPrepaidTraffic(ctx context.Context) float64
}

type alibaba struct {
	cfg            *config.Config
	client         *bssopenapi.Client
	client20171214 *bssopenapi20171214.Client
	counters       *metrics.Counters
}

func NewAlibaba(cfg *config.Config, counters *metrics.Counters) (Alibaba, error) {
	sdkConfig := sdk.NewConfig()
	sdkConfig.Scheme = "https"
	cred := credentials.NewAccessKeyCredential(cfg.AlibabaConf.AccessKeyId, string(cfg.AlibabaConf.AccessKeySecret))
	client, _err := bssopenapi.NewClientWithOptions(cfg.AlibabaConf.RegionId, sdkConfig, cred)
	if _err != nil {
		return nil, _err
	}

	config := &openapi.Config{
		AccessKeyId:     tea.String(cfg.AlibabaConf.AccessKeyId),
		AccessKeySecret: tea.String(string(cfg.AlibabaConf.AccessKeySecret)),
	}
	// See https://api.alibabacloud.com/product/BssOpenApi.
	config.Endpoint = tea.String("business.ap-southeast-1.aliyuncs.com")
	client20171214 := &bssopenapi20171214.Client{}
	client20171214, _err = bssopenapi20171214.NewClient(config)

	if _err != nil {
		return nil, _err
	}

	alibaba := &alibaba{
		cfg:            cfg,
		client:         client,
		client20171214: client20171214,
		counters:       counters,
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
func (a *alibaba) GetPrepaidTraffic(ctx context.Context) (totalTraffic float64) {
	totalTraffic = 0
	_pageSize := int32(100)
	request := &bssopenapi20171214.QueryResourcePackageInstancesRequest{
		PageSize: &_pageSize,
	}
	// TODO getAllPages
	runtime := &util.RuntimeOptions{}
	if _result, _err := a.client20171214.QueryResourcePackageInstancesWithOptions(request, runtime); _err != nil {
		zap.S().Warn(_err)
		return
	} else {
		for i := range _result.Body.Data.Instances.Instance {
			instance := _result.Body.Data.Instances.Instance[i]
			zap.S().Debugf("Instance(%s): %s %s (%s)", *instance.InstanceId, *instance.RemainingAmount, *instance.RemainingAmountUnit, *instance.Status)
			if *instance.Status == "Available" {
				size, _ := strconv.ParseFloat(*instance.RemainingAmount, 64)
				totalTraffic += size * unit2multi(*instance.RemainingAmountUnit)
				zap.S().Debugf("%f total:  %f", size, totalTraffic)
			}
		}
	}
	return
}

const KB = 1024
const MB = 1024 * KB
const GB = 1024 * MB
const TB = 1024 * GB

func unit2multi(unit string) float64 {
	switch unit {
	case "TB":
		return TB
	case "GB":
		return GB
	case "MB":
		return MB
	case "KB":
		return KB
	case "Byte":
		return 1
	}
	return 0
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
	a.counters.PrepaidTraffic.Set(a.GetPrepaidTraffic(ctx))
	a.counters.AvailableAmount.Set(a.GetAccountBalance(ctx))
	makeVector(a.counters.TotalInstances, a.GetAllAvailableInstance(ctx))
	a.GetDiskList(ctx)
	elapsed := time.Since(start)
	zap.S().Infof("%s finish getAlibaba (took %s)", time.Now(), elapsed)
}
