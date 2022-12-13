package producer

import (
	"strings"
	"time"

	"github.com/free5gc/CDRUtil/asn"
	"github.com/free5gc/CDRUtil/cdrConvert"
	"github.com/free5gc/CDRUtil/cdrFile"
	"github.com/free5gc/CDRUtil/cdrType"
	chf_context "github.com/free5gc/chf/internal/context"
	"github.com/free5gc/chf/internal/logger"
	"github.com/free5gc/openapi/models"
)

func OpenCDR(chargingData models.ChargingDataRequest, supi string, sessionId string, partialRecord bool) (*cdrType.CHFRecord, error) {
	// 32.298 5.1.5.0.1 for CHF CDR field
	var chfCdr cdrType.ChargingRecord

	chfCdr.RecordType = cdrType.RecordType{
		Value: 200,
	}

	// TODO IA5 string coversion
	self := chf_context.CHF_Self()
	chfCdr.RecordingNetworkFunctionID = cdrType.NetworkFunctionName{
		Value: asn.IA5String(self.NfId),
	}

	// RecordOpeningTime: Time stamp when the PDU session is activated in the SMF or record opening time on subsequent partial records.
	// TODO identify charging event is SMF PDU session
	t := time.Now()
	chfCdr.RecordOpeningTime = cdrConvert.TimeStampToCdr(&t)

	// Initial CDR duration
	chfCdr.Duration = cdrType.CallDuration{
		Value: 0,
	}

	// Record Sequence Number(Conditional IE): Partial record sequence number, only present in case of partial records.
	// Partial CDR: Fragments of CDR, for long session charging
	if partialRecord {
		// TODO partial record
		var partialRecordSeqNum int64
		chfCdr.RecordSequenceNumber = &partialRecordSeqNum
	}

	// 32.298 5.1.5.1.5 Local Record Sequence Number
	// TODO determine local record sequnece number
	self.LocalRecordSequenceNumber++
	chfCdr.LocalRecordSequenceNumber = &cdrType.LocalSequenceNumber{
		Value: int64(self.LocalRecordSequenceNumber),
	}
	// Skip Record Extensions: operator/manufacturer specific extensions

	supiType := strings.Split(supi, "-")[0]
	switch supiType {
	case "imsi":
		logger.ChargingdataPostLog.Debugf("SUPI: %s", supi)
		chfCdr.SubscriberIdentifier = &cdrType.SubscriptionID{
			SubscriptionIDType: cdrType.SubscriptionIDType{Value: cdrType.SubscriptionIDTypePresentENDUSERIMSI},
			SubscriptionIDData: asn.UTF8String(supi[5:]),
		}
	case "nai":
		chfCdr.SubscriberIdentifier = &cdrType.SubscriptionID{
			SubscriptionIDType: cdrType.SubscriptionIDType{Value: cdrType.SubscriptionIDTypePresentENDUSERNAI},
			SubscriptionIDData: asn.UTF8String(supi[4:]),
		}
	case "gci":
		chfCdr.SubscriberIdentifier = &cdrType.SubscriptionID{
			SubscriptionIDType: cdrType.SubscriptionIDType{Value: cdrType.SubscriptionIDTypePresentENDUSERNAI},
			SubscriptionIDData: asn.UTF8String(supi[4:]),
		}
	case "gli":
		chfCdr.SubscriberIdentifier = &cdrType.SubscriptionID{
			SubscriptionIDType: cdrType.SubscriptionIDType{Value: cdrType.SubscriptionIDTypePresentENDUSERNAI},
			SubscriptionIDData: asn.UTF8String(supi[4:]),
		}
	}

	if sessionId != "" {
		chfCdr.ChargingSessionIdentifier = &cdrType.ChargingSessionIdentifier{
			Value: asn.OctetString(sessionId),
		}
	}

	chfCdr.ChargingID = &cdrType.ChargingID{
		Value: int64(chargingData.ChargingId),
	}

	var consumerInfo cdrType.NetworkFunctionInformation
	if consumerName := chargingData.NfConsumerIdentification.NFName; consumerName != "" {
		consumerInfo.NetworkFunctionName = &cdrType.NetworkFunctionName{
			Value: asn.IA5String(chargingData.NfConsumerIdentification.NFName),
		}
	}
	if consumerV4Addr := chargingData.NfConsumerIdentification.NFIPv4Address; consumerV4Addr != "" {
		consumerInfo.NetworkFunctionIPv4Address = &cdrType.IPAddress{
			Present:         3,
			IPTextV4Address: (*asn.IA5String)(&consumerV4Addr),
		}
	}
	if consumerV6Addr := chargingData.NfConsumerIdentification.NFIPv6Address; consumerV6Addr != "" {
		consumerInfo.NetworkFunctionIPv6Address = &cdrType.IPAddress{
			Present:         4,
			IPTextV6Address: (*asn.IA5String)(&consumerV6Addr),
		}
	}
	if consumerFqdn := chargingData.NfConsumerIdentification.NFFqdn; consumerFqdn != "" {
		consumerInfo.NetworkFunctionFQDN = &cdrType.NodeAddress{
			Present:    2,
			DomainName: (*asn.GraphicString)(&consumerFqdn),
		}
	}
	if consumerPlmnId := chargingData.NfConsumerIdentification.NFPLMNID; consumerPlmnId != nil {
		plmnIdByte := cdrConvert.PlmnIdToCdr(*consumerPlmnId)
		consumerInfo.NetworkFunctionPLMNIdentifier = &cdrType.PLMNId{
			Value: plmnIdByte.Value,
		}

	}
	logger.ChargingdataPostLog.Infof("%s charging event", chargingData.NfConsumerIdentification.NodeFunctionality)
	switch chargingData.NfConsumerIdentification.NodeFunctionality {
	case "SMF":
		consumerInfo.NetworkFunctionality.Value = cdrType.NetworkFunctionalityPresentSMF
	case "AMF":
		consumerInfo.NetworkFunctionality.Value = cdrType.NetworkFunctionalityPresentAMF
	case "SMSF":
		consumerInfo.NetworkFunctionality.Value = cdrType.NetworkFunctionalityPresentSMSF
	case "PGW_C_SMF":
		consumerInfo.NetworkFunctionality.Value = cdrType.NetworkFunctionalityPresentPGWCSMF
	case "NEF":
		consumerInfo.NetworkFunctionality.Value = cdrType.NetworkFunctionalityPresentNEF
	case "SGW":
		consumerInfo.NetworkFunctionality.Value = cdrType.NetworkFunctionalityPresentSGW
	case "I_SMF":
		consumerInfo.NetworkFunctionality.Value = cdrType.NetworkFunctionalityPresentISMF
	case "ePDG":
		consumerInfo.NetworkFunctionality.Value = cdrType.NetworkFunctionalityPresentEPDG
	case "CEF":
		consumerInfo.NetworkFunctionality.Value = cdrType.NetworkFunctionalityPresentCEF
	case "MnS_Producer":
		consumerInfo.NetworkFunctionality.Value = cdrType.NetworkFunctionalityPresentMnSProducer
	}
	chfCdr.NFunctionConsumerInformation = consumerInfo

	if serviceSpecInfo := asn.OctetString(chargingData.ServiceSpecificationInfo); len(serviceSpecInfo) != 0 {
		chfCdr.ServiceSpecificationInformation = &serviceSpecInfo
	}

	// TODO: encode service specific data to CDR
	if registerInfo := chargingData.RegistrationChargingInformation; registerInfo != nil {
		logger.ChargingdataPostLog.Debugln("Registration Charging Event")
		chfCdr.RegistrationChargingInformation = &cdrType.RegistrationChargingInformation{
			RegistrationMessagetype: cdrType.RegistrationMessageType{Value: cdrType.RegistrationMessageTypePresentInitial},
		}
	}
	if pduSessionInfo := chargingData.PDUSessionChargingInformation; pduSessionInfo != nil {
		logger.ChargingdataPostLog.Debugln("PDU Session Charging Event")
		chfCdr.PDUSessionChargingInformation = &cdrType.PDUSessionChargingInformation{
			PDUSessionChargingID: cdrType.ChargingID{
				Value: int64(pduSessionInfo.ChargingId),
			},
			PDUSessionId: cdrType.PDUSessionId{
				Value: int64(pduSessionInfo.PduSessionInformation.PduSessionID),
			},
		}
	}

	cdr := cdrType.CHFRecord{
		Present:                1,
		ChargingFunctionRecord: &chfCdr,
	}

	return &cdr, nil
}

func UpdateCDR(record *cdrType.CHFRecord, chargingData models.ChargingDataRequest, sessionId string, partialRecord bool) error {
	// map SBI IE to CDR field
	chfCdr := record.ChargingFunctionRecord

	if len(chargingData.MultipleUnitUsage) != 0 {
		// NOTE: quota info needn't be encoded to cdr, refer 32.291 Ch7.1
		cdrMultiUnitUsage := cdrConvert.MultiUnitUsageToCdr(chargingData.MultipleUnitUsage)
		chfCdr.ListOfMultipleUnitUsage = append(chfCdr.ListOfMultipleUnitUsage, cdrMultiUnitUsage...)
	}

	if len(chargingData.Triggers) != 0 {
		triggers := cdrConvert.TriggersToCdr(chargingData.Triggers)
		chfCdr.Triggers = append(chfCdr.Triggers, triggers...)
	}

	return nil
}

func CloseCDR(record *cdrType.CHFRecord, partial bool) error {
	chfCdr := record.ChargingFunctionRecord

	// Initial Cause for record closing
	// 	normalRelease  (0),
	// partialRecord  (1),
	// abnormalRelease  (4),
	// cAMELInitCallRelease  (5),
	// volumeLimit	 (16),
	// timeLimit	 (17),
	// servingNodeChange	 (18),
	// maxChangeCond	 (19),
	// managementIntervention	 (20),
	// intraSGSNIntersystemChange	 (21),
	// rATChange	 (22),
	// mSTimeZoneChange	 (23),
	// sGSNPLMNIDChange	 (24),
	// sGWChange	 (25),
	// aPNAMBRChange	 (26),
	// mOExceptionDataCounterReceipt	 (27),
	// unauthorizedRequestingNetwork	 (52),
	// unauthorizedLCSClient	 (53),
	// positionMethodFailure	 (54),
	// unknownOrUnreachableLCSClient	 (58),
	// listofDownstreamNodeChange	 (59)
	if partial {
		chfCdr.CauseForRecClosing = cdrType.CauseForRecClosing{Value: 1}
	} else {
		chfCdr.CauseForRecClosing = cdrType.CauseForRecClosing{Value: 0}
	}

	return nil
}

func dumpCdrFile(ueid string, records []*cdrType.CHFRecord) error {
	logger.ChargingdataPostLog.Tracef("Dump CDR File")

	var cdrfile cdrFile.CDRFile
	cdrfile.Hdr.LengthOfCdrRouteingFilter = 0
	cdrfile.Hdr.LengthOfPrivateExtension = 0
	cdrfile.Hdr.HeaderLength = uint32(54 + cdrfile.Hdr.LengthOfCdrRouteingFilter + cdrfile.Hdr.LengthOfPrivateExtension)
	cdrfile.Hdr.NumberOfCdrsInFile = uint32(len(records))
	cdrfile.Hdr.FileLength = cdrfile.Hdr.HeaderLength

	for _, record := range records {
		cdrBytes, err := asn.BerMarshalWithParams(&record, "explicit,choice")
		if err != nil {
			logger.ChargingdataPostLog.Errorln(err)
		}

		var cdrHdr cdrFile.CdrHeader
		cdrHdr.CdrLength = uint16(len(cdrBytes))
		cdrHdr.DataRecordFormat = cdrFile.BasicEncodingRules
		tmpCdr := cdrFile.CDR{
			Hdr:     cdrHdr,
			CdrByte: cdrBytes,
		}
		cdrfile.CdrList = append(cdrfile.CdrList, tmpCdr)

		cdrfile.Hdr.FileLength += uint32(cdrHdr.CdrLength) + 5
	}

	cdrfile.Encoding("/tmp/" + ueid + ".cdr")

	return nil
}
