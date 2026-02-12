package alibaba

import (
	"alibaba-exporter/config"
	"alibaba-exporter/metrics"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	bssopenapi20171214 "github.com/alibabacloud-go/bssopenapi-20171214/v6/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecs20140526 "github.com/alibabacloud-go/ecs-20140526/v7/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/bssopenapi"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

type Alibaba interface {
	GetAccountBalance(ctx context.Context) float64
	MainLoop(ctx context.Context)
	GetPrepaidCommodities(ctx context.Context) map[string]float64
	GetRegions(ctx context.Context) []string
}

type alibaba struct {
	cfg            *config.Config
	client         *bssopenapi.Client
	client20171214 *bssopenapi20171214.Client
	counters       *metrics.Counters
}

func NewAlibaba(cfg *config.Config, counters *metrics.Counters) (Alibaba, error) {
	// Конфиг для V2 SDK (Darabonba)
	configV2 := &openapi.Config{
		AccessKeyId:     tea.String(cfg.AlibabaConf.AccessKeyId),
		AccessKeySecret: tea.String(string(cfg.AlibabaConf.AccessKeySecret)),
		Endpoint:        tea.String("business.ap-southeast-1.aliyuncs.com"),
	}
	clientV2, err := bssopenapi20171214.NewClient(configV2)
	if err != nil {
		return nil, fmt.Errorf("failed to init v2 client: %w", err)
	}

	// Конфиг для старого SDK
	cred := credentials.NewAccessKeyCredential(cfg.AlibabaConf.AccessKeyId, string(cfg.AlibabaConf.AccessKeySecret))
	clientV1, err := bssopenapi.NewClientWithOptions(cfg.AlibabaConf.RegionId, sdk.NewConfig(), cred)
	if err != nil {
		return nil, fmt.Errorf("failed to init v1 client: %w", err)
	}

	return &alibaba{
		cfg:            cfg,
		client:         clientV1,
		client20171214: clientV2,
		counters:       counters,
	}, nil
}

func (a *alibaba) GetAccountBalance(ctx context.Context) float64 {
	if err := ctx.Err(); err != nil {
		zap.S().Warnf("GetPrepaidCommodities interrupted: %v", err)
		return 0
	}

	res, err := a.client20171214.QueryAccountBalance()
	if err != nil {
		zap.S().Errorf("V2 Balance API error: %v", err)
		return 0
	}

	if res.Body.Data == nil || res.Body.Data.AvailableAmount == nil {
		return 0
	}

	amountStr := strings.ReplaceAll(*res.Body.Data.AvailableAmount, ",", "")
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		zap.S().Warnf("Failed to parse balance %s: %v", amountStr, err)
		return 0
	}
	return amount
}

func (a *alibaba) GetPrepaidCommodities(ctx context.Context) map[string]float64 {
	commodities := make(map[string]float64)
	pageNum := int32(1)
	pageSize := int32(100)

	for {
		if err := ctx.Err(); err != nil {
			zap.S().Warnf("GetPrepaidCommodities interrupted: %v", err)
			return commodities
		}

		request := &bssopenapi20171214.QueryResourcePackageInstancesRequest{
			PageNum:  &pageNum,
			PageSize: &pageSize,
		}

		runtime := &util.RuntimeOptions{}
		result, err := a.client20171214.QueryResourcePackageInstancesWithOptions(request, runtime)
		if err != nil {
			zap.S().Errorf("Failed to fetch resource packages: %v", err)
			return commodities
		}

		// Проверка на наличие данных в ответе
		if result.Body.Data == nil || result.Body.Data.Instances == nil {
			break
		}

		instances := result.Body.Data.Instances.Instance
		for _, inst := range instances {
			status := tea.StringValue(inst.Status)
			id := tea.StringValue(inst.InstanceId)
			remAmount := tea.StringValue(inst.RemainingAmount)
			unit := tea.StringValue(inst.RemainingAmountUnit)
			code := tea.StringValue(inst.CommodityCode)

			zap.S().Debugf("Instance(%s): %s %s (%s)", id, remAmount, unit, status)

			if status == "Available" && code != "" {
				size, err := strconv.ParseFloat(remAmount, 64)
				if err != nil {
					continue
				}

				totalValue := size * unit2multi(unit)
				commodities[code] += totalValue

				zap.S().Debugf("%s - %f (converted: %f) total: %f", code, size, totalValue, commodities[code])
			}
		}

		if len(instances) < int(pageSize) {
			break
		}
		pageNum++
	}
	return commodities
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

func (a *alibaba) GetAllECSInstances(ctx context.Context, regionID string) []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance {
	var allInstances []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance

	config := &openapi.Config{
		AccessKeyId:     tea.String(a.cfg.AlibabaConf.AccessKeyId),
		AccessKeySecret: tea.String(string(a.cfg.AlibabaConf.AccessKeySecret)),
		RegionId:        tea.String(regionID),
	}

	client, err := ecs20140526.NewClient(config)
	if err != nil {
		zap.S().Errorf("Failed to create ECS V2 client for region %s: %v", regionID, err)
		return nil
	}

	pageNum := int32(1)
	pageSize := int32(100)

	for {
		select {
		case <-ctx.Done():
			zap.S().Warnf("GetAllECSInstances(%s) interrupted by context", regionID)
			return allInstances
		default:
		}

		request := &ecs20140526.DescribeInstancesRequest{
			RegionId:   tea.String(regionID),
			PageNumber: tea.Int32(pageNum),
			PageSize:   tea.Int32(pageSize),
		}

		runtime := &util.RuntimeOptions{}
		resp, err := client.DescribeInstancesWithOptions(request, runtime)
		if err != nil {
			zap.S().Errorf("V2 DescribeInstances error in region %s: %v", regionID, err)
			break
		}

		if resp.Body == nil || resp.Body.Instances == nil {
			break
		}

		instances := resp.Body.Instances.Instance
		allInstances = append(allInstances, instances...)

		totalCount := tea.Int32Value(resp.Body.TotalCount)
		if len(allInstances) >= int(totalCount) || len(instances) == 0 {
			break
		}

		pageNum++
	}

	zap.S().Debugf("Region %s: total instances fetched: %d", regionID, len(allInstances))
	return allInstances
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

func (a *alibaba) GetRegions(ctx context.Context) []string {
	var regionIDs []string

	cfg := &openapi.Config{
		AccessKeyId:     tea.String(a.cfg.AlibabaConf.AccessKeyId),
		AccessKeySecret: tea.String(string(a.cfg.AlibabaConf.AccessKeySecret)),
		RegionId:        tea.String(a.cfg.AlibabaConf.RegionId),
	}

	client, err := ecs20140526.NewClient(cfg)
	if err != nil {
		zap.S().Errorf("Failed to init ECS client for regions: %v", err)
		return nil
	}

	req := &ecs20140526.DescribeRegionsRequest{}
	res, err := client.DescribeRegionsWithOptions(req, &util.RuntimeOptions{})
	if err != nil {
		zap.S().Errorf("Failed to fetch regions via V2: %v", err)
		return nil
	}

	if res.Body == nil || res.Body.Regions == nil {
		return nil
	}

	for _, reg := range res.Body.Regions.Region {
		if reg.RegionId != nil {
			regionIDs = append(regionIDs, *reg.RegionId)
		}
	}

	return regionIDs
}

func makeVector(vector *prometheus.GaugeVec, instanceList []bssopenapi.Instance) {
	vector.Reset()
	for _, inst := range instanceList {
		vector.WithLabelValues(inst.ProductCode, inst.SubscriptionType, inst.Region, inst.RenewStatus, inst.Status, inst.SubStatus).Add(1)
	}
}

func (a *alibaba) MainLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * time.Duration(a.cfg.Interval))
	defer ticker.Stop()

	a.getAlibabaMetrics(ctx)
	for {
		select {
		case <-ctx.Done():
			zap.S().Info("MainLoop: context cancelled, shutting down")
			return
		case _ = <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						zap.S().Errorf("Recovered from panic in getAlibabaMetrics: %v", r)
					}
				}()
				a.getAlibabaMetrics(ctx)
			}()
		}
	}
}

func (a *alibaba) getAlibabaMetrics(ctx context.Context) {
	start := time.Now()
	zap.S().Infof("%s start getAlibaba", start)

	commodities := a.GetPrepaidCommodities(ctx)
	a.counters.PrepaidCommodities.Reset()
	for commodity, value := range commodities {
		a.counters.PrepaidCommodities.WithLabelValues(commodity).Set(value)
	}
	a.counters.PrepaidTraffic.Set(commodities["flowbag_intl"])
	a.counters.AvailableAmount.Set(a.GetAccountBalance(ctx))

	regions := a.GetRegions(ctx)
	var allRegionsInstances []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance
	for _, regionID := range regions {
		if strings.HasPrefix(regionID, "cn") {
			zap.S().Debugf("Skipping region: %s", regionID)
			continue
		}
		instances := a.GetAllECSInstances(ctx, regionID)
		allRegionsInstances = append(allRegionsInstances, instances...)
	}
	var ram, cpu float64
	for _, instance := range allRegionsInstances {
		cpu += float64(*instance.Cpu)
		ram += float64(*instance.Memory)
	}
	a.counters.EcsCpu.Set(cpu)
	a.counters.EcsRam.Set(ram)
	makeEcsVector(a.counters.EcsInstances, allRegionsInstances)

	makeVector(a.counters.TotalInstances, a.GetAllAvailableInstance(ctx))
	a.GetDiskList(ctx)
	elapsed := time.Since(start)
	zap.S().Infof("%s finish getAlibaba (took %s)", time.Now(), elapsed)
}

func makeEcsVector(vector *prometheus.GaugeVec, instanceList []*ecs20140526.DescribeInstancesResponseBodyInstancesInstance) {
	vector.Reset()
	for _, inst := range instanceList {
		workload := "unknown"
		if inst.GetTags() != nil {
			tags := (*inst.GetTags()).GetTag()
			for _, tag := range tags {
				if *tag.TagKey == "workload" {
					workload = *tag.TagValue
				}
			}
		}
		vector.WithLabelValues(*inst.GetRegionId(), *inst.GetInstanceChargeType(),
			*inst.GetInstanceType(), workload).Add(1)
	}
}

func unit2multi(unit string) float64 {
	switch unit {
	case "TB":
		return 1024 * 1024 * 1024 * 1024
	case "GB":
		return 1024 * 1024 * 1024
	case "MB":
		return 1024 * 1024
	case "KB":
		return 1024
	default:
		return 1
	}
}
