package rating

import (
	"math"
	"strings"
	"time"

	"github.com/free5gc/TarrifUtil/asn"
	"github.com/free5gc/TarrifUtil/tarrifType"
	chf_context "github.com/free5gc/chf/internal/context"
	"github.com/free5gc/chf/internal/logger"
	"github.com/free5gc/openapi/models"
	"github.com/free5gc/util/mongoapi"
	"go.mongodb.org/mongo-driver/bson"
)

const chargingDataColl = "chargingData"

func ServiceUsageRetrieval(serviceUsage tarrifType.ServiceUsageRequest) (tarrifType.ServiceUsageResponse, *models.ProblemDetails, bool) {
	lastgrantedquota := false

	unitCost := (serviceUsage.ServiceRating.CurrentTariff.RateElement.UnitCost.ValueDigits) * int64(math.Pow10(int(serviceUsage.ServiceRating.CurrentTariff.RateElement.UnitCost.Exponent)))
	monetaryCost := int64(serviceUsage.ServiceRating.ConsumedUnits) * unitCost
	monetaryRequest := int64(serviceUsage.ServiceRating.RequestedUnits) * unitCost

	logger.ChargingdataPostLog.Tracef("Cost per Byte[%d]", unitCost)
	rsp := tarrifType.ServiceUsageResponse{
		SessionID: serviceUsage.SessionID,
		ServiceRating: &tarrifType.ServiceRating{
			Price:         uint32(monetaryCost),
			MonetaryQuota: serviceUsage.ServiceRating.MonetaryQuota,
		},
	}

	if serviceUsage.ServiceRating.RequestSubType.Value == tarrifType.REQ_SUBTYPE_DEBIT {
		logger.ChargingdataPostLog.Warnf("Out of Monetary Quota, Debit mode")
		rsp.ServiceRating.AllowedUnits = 0
		return rsp, nil, lastgrantedquota
	} else if serviceUsage.ServiceRating.RequestSubType.Value == tarrifType.REQ_SUBTYPE_RESERVE {
		if monetaryCost < int64(serviceUsage.ServiceRating.MonetaryQuota) {
			monetaryRemain := int64(serviceUsage.ServiceRating.MonetaryQuota) - monetaryCost
			if (monetaryRemain - monetaryRequest) > 0 {
				rsp.ServiceRating.AllowedUnits = serviceUsage.ServiceRating.RequestedUnits
			} else {
				rsp.ServiceRating.AllowedUnits = uint32(monetaryRemain / unitCost)
				logger.ChargingdataPostLog.Warn("Last granted Quota")
				lastgrantedquota = true
			}
		} else {
			logger.ChargingdataPostLog.Warn("Out of Monetary Quota")
			rsp.ServiceRating.AllowedUnits = 0
			return rsp, nil, lastgrantedquota
		}
	} else {
		logger.ChargingdataPostLog.Warnf("Unsupport RequestSubType")
	}

	return rsp, nil, lastgrantedquota
}

func BuildServiceUsageRequest(chargingData models.ChargingDataRequest, unitUsage models.MultipleUnitUsage, sessionid int64, ratingGroup int32) tarrifType.ServiceUsageRequest {
	supi := chargingData.SubscriberIdentifier
	supiType := strings.Split(supi, "-")[0]
	var subscriberIdentifier tarrifType.SubscriptionID

	switch supiType {
	case "imsi":
		subscriberIdentifier = tarrifType.SubscriptionID{
			SubscriptionIDType: &tarrifType.SubscriptionIDType{Value: tarrifType.END_USER_IMSI},
			SubscriptionIDData: asn.UTF8String(supi[5:]),
		}
	case "nai":
		subscriberIdentifier = tarrifType.SubscriptionID{
			SubscriptionIDType: &tarrifType.SubscriptionIDType{Value: tarrifType.END_USER_NAI},
			SubscriptionIDData: asn.UTF8String(supi[4:]),
		}
	case "gci":
		subscriberIdentifier = tarrifType.SubscriptionID{
			SubscriptionIDType: &tarrifType.SubscriptionIDType{Value: tarrifType.END_USER_NAI},
			SubscriptionIDData: asn.UTF8String(supi[4:]),
		}
	case "gli":
		subscriberIdentifier = tarrifType.SubscriptionID{
			SubscriptionIDType: &tarrifType.SubscriptionIDType{Value: tarrifType.END_USER_NAI},
			SubscriptionIDData: asn.UTF8String(supi[4:]),
		}
	}

	// Rating for each rating group
	var totalUsaedUnit uint32
	for _, useduint := range unitUsage.UsedUnitContainer {
		if useduint.QuotaManagementIndicator == models.QuotaManagementIndicator_OFFLINE_CHARGING {
			continue
		}

		totalUsaedUnit += uint32(useduint.TotalVolume)
	}

	self := chf_context.CHF_Self()
	ue, ok := self.ChfUeFindBySupi(supi)
	if ok {
		ue.AccumulateUsage.TotalVolume += int32(totalUsaedUnit)
		logger.ChargingdataPostLog.Warnf("UE's[%s] accumulate data usage %d", supi, ue.AccumulateUsage.TotalVolume)
	}

	filter := bson.M{"ueId": chargingData.SubscriberIdentifier, "ratingGroup": 1}
	chargingInterface, err := mongoapi.RestfulAPIGetOne(chargingDataColl, filter)
	if err != nil {
		logger.ChargingdataPostLog.Errorf("Get quota error: %+v", err)
	}

	// workaround
	// type reading from mongoDB is not stabe
	// i.g. chargingInterface["quota"] may be int, float...
	// 		tarrifInterface["rateelement"] may be tarrifInterface["rateElement"]

	quota := uint32(0)
	switch value := chargingInterface["quota"].(type) {
	case int:
		quota = uint32(value)
	case int32:
		quota = uint32(value)
	case int64:
		quota = uint32(value)
	case float64:
		quota = uint32(value)
	default:
		logger.ChargingdataPostLog.Errorf("Get quota error: do not belong to int or float, type:%T", chargingInterface["quota"])
	}

	filter = bson.M{"ueId": chargingData.SubscriberIdentifier, "ratingGroup": ratingGroup}
	chargingInterface, err = mongoapi.RestfulAPIGetOne(chargingDataColl, filter)
	if err != nil {
		logger.ChargingdataPostLog.Errorf("Get quota error: %+v", err)
	}

	tarrifInterface := chargingInterface["tarrif"].(map[string]interface{})

	// logger.ChargingdataPostLog.Errorf("Please check if the tarrifInterface exactly contains rateelement/unitcost or rateElement/unitCost if error occurs")
	// logger.ChargingdataPostLog.Warnf("tarrifInterface %+v", tarrifInterface)

	rateElementInterface := make(map[string]interface{})
	if tarrifInterface["rateElement"] == nil {
		rateElementInterface = tarrifInterface["rateelement"].(map[string]interface{})
	} else {
		rateElementInterface = tarrifInterface["rateElement"].(map[string]interface{})
	}

	unitCostInterface := make(map[string]interface{})
	if rateElementInterface["unitCost"] == nil {
		unitCostInterface = rateElementInterface["unitcost"].(map[string]interface{})
	} else {
		unitCostInterface = rateElementInterface["unitCost"].(map[string]interface{})
	}

	// workaround
	exponent := int32(0)
	switch value := unitCostInterface["exponent"].(type) {
	case int:
		exponent = int32(value)
	case int32:
		exponent = int32(value)
	case int64:
		exponent = int32(value)
	case float64:
		exponent = int32(value)
	default:
		logger.ChargingdataPostLog.Errorf("Get exponent error: do not belong to int or float, type:%T", unitCostInterface["exponent"])
	}

	var valueDigitsString string
	if unitCostInterface["valueDigits"] == nil {
		valueDigitsString = "valuedigits"
	} else {
		valueDigitsString = "valueDigits"
	}

	valueDigits := int64(0)
	switch value := unitCostInterface[valueDigitsString].(type) {
	case int:
		valueDigits = int64(value)
	case int32:
		valueDigits = int64(value)
	case int64:
		valueDigits = int64(value)
	case float64:
		valueDigits = int64(value)
	default:
		logger.ChargingdataPostLog.Errorf("Get valueDigits error: do not belong to int or float, type:%T", unitCostInterface["valueDigits"])
	}
	tarrif := tarrifType.CurrentTariff{
		RateElement: &tarrifType.RateElement{
			UnitCost: &tarrifType.UnitCost{
				Exponent:    int(exponent),
				ValueDigits: valueDigits,
			},
		},
	}

	ServiceUsageRequest := tarrifType.ServiceUsageRequest{
		SessionID:      int(sessionid),
		SubscriptionID: &subscriberIdentifier,
		ActualTime:     time.Now(),
		ServiceRating: &tarrifType.ServiceRating{
			RequestedUnits: uint32(unitUsage.RequestedUnit.TotalVolume),
			ConsumedUnits:  totalUsaedUnit,
			RequestSubType: &tarrifType.RequestSubType{
				Value: tarrifType.REQ_SUBTYPE_RESERVE,
			},
			CurrentTariff: &tarrif,
			MonetaryQuota: quota,
		},
	}
	if quota == 0 {
		ServiceUsageRequest.ServiceRating.RequestSubType.Value = tarrifType.REQ_SUBTYPE_DEBIT
	}

	for _, trigger := range chargingData.Triggers {
		if trigger.TriggerType == models.TriggerType_FINAL {
			ServiceUsageRequest.ServiceRating.RequestSubType.Value = tarrifType.REQ_SUBTYPE_DEBIT
		}
	}
	return ServiceUsageRequest
}
