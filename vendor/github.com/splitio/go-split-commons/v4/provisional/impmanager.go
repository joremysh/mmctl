package provisional

import (
	"time"

	"github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-split-commons/v4/telemetry"
	"github.com/splitio/go-split-commons/v4/util"
)

const lastSeenCacheSize = 500000 // cache up to 500k impression hashes

// ImpressionManager interface
type ImpressionManager interface {
	ProcessImpressions(impressions []dtos.Impression) ([]dtos.Impression, []dtos.Impression)
	ProcessSingle(impression *dtos.Impression) (toLog bool, toListener bool)
}

// ImpressionManagerImpl implements
type ImpressionManagerImpl struct {
	impressionObserver    ImpressionObserver
	impressionsCounter    *ImpressionsCounter
	shouldAddPreviousTime bool
	isOptimized           bool
	listenerEnabled       bool
	runtimeTelemetry      storage.TelemetryRuntimeProducer
}

// NewImpressionManager creates new ImpManager
func NewImpressionManager(managerConfig conf.ManagerConfig, impressionCounter *ImpressionsCounter, runtimeTelemetry storage.TelemetryRuntimeProducer) (ImpressionManager, error) {
	impressionObserver, err := NewImpressionObserver(lastSeenCacheSize)
	if err != nil {
		return nil, err
	}

	impManager := &ImpressionManagerImpl{
		impressionObserver:    impressionObserver,
		impressionsCounter:    impressionCounter,
		shouldAddPreviousTime: util.ShouldAddPreviousTime(managerConfig),
		isOptimized:           impressionCounter != nil && util.ShouldBeOptimized(managerConfig),
		listenerEnabled:       managerConfig.ListenerEnabled,
		runtimeTelemetry:      runtimeTelemetry,
	}

	return impManager, nil
}

func (i *ImpressionManagerImpl) processImpression(impression dtos.Impression, forLog []dtos.Impression, forListener []dtos.Impression) ([]dtos.Impression, []dtos.Impression) {
	if i.shouldAddPreviousTime {
		impression.Pt, _ = i.impressionObserver.TestAndSet(impression.FeatureName, &impression) // Adds previous time if it is enabled
	}

	now := time.Now().UTC().UnixNano()
	if i.isOptimized { // isOptimized
		i.impressionsCounter.Inc(impression.FeatureName, now, 1) // Increments impression counter per featureName
	}

	if !i.isOptimized || impression.Pt == 0 || impression.Pt < util.TruncateTimeFrame(now) {
		forLog = append(forLog, impression)
	}

	if i.listenerEnabled {
		forListener = append(forListener, impression)
	}

	return forLog, forListener
}

// ProcessImpressions bulk processes
func (i *ImpressionManagerImpl) ProcessImpressions(impressions []dtos.Impression) ([]dtos.Impression, []dtos.Impression) {
	forLog := make([]dtos.Impression, 0, len(impressions))
	forListener := make([]dtos.Impression, 0, len(impressions))

	for _, impression := range impressions {
		forLog, forListener = i.processImpression(impression, forLog, forListener)
	}

	i.runtimeTelemetry.RecordImpressionsStats(telemetry.ImpressionsDeduped, int64(len(impressions)-len(forLog)))
	return forLog, forListener
}

// ProcessSingle accepts a pointer to an impression, updates it's PT accordingly,
// and returns whether it should be sent to the BE and to the lister
func (i *ImpressionManagerImpl) ProcessSingle(impression *dtos.Impression) (toLog bool, toListener bool) {
	if i.shouldAddPreviousTime {
		impression.Pt, _ = i.impressionObserver.TestAndSet(impression.FeatureName, impression) // Adds previous time if it is enabled
	}

	now := time.Now().UTC().UnixNano()
	if i.isOptimized { // isOptimized
		i.impressionsCounter.Inc(impression.FeatureName, now, 1) // Increments impression counter per featureName
	}

	return !i.isOptimized || impression.Pt == 0 || impression.Pt < util.TruncateTimeFrame(now), i.listenerEnabled
}
