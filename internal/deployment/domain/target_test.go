package domain_test

import (
	"errors"
	"testing"
	"time"

	auth "github.com/YuukanOO/seelf/internal/auth/domain"
	"github.com/YuukanOO/seelf/internal/deployment/domain"
	"github.com/YuukanOO/seelf/pkg/must"
	"github.com/YuukanOO/seelf/pkg/testutil"
)

func Test_Target(t *testing.T) {
	var (
		name                            = "my-target"
		targetUrl                       = must.Panic(domain.UrlFrom("http://my-url.com"))
		config    domain.ProviderConfig = dummyProviderConfig{}
		uid       auth.UserID           = "uid"

		urlNotUnique    = domain.NewTargetUrlRequirement(targetUrl, false)
		urlUnique       = domain.NewTargetUrlRequirement(targetUrl, true)
		configNotUnique = domain.NewProviderConfigRequirement(config, false)
		configUnique    = domain.NewProviderConfigRequirement(config, true)
	)

	t.Run("should fail if the url is not unique", func(t *testing.T) {
		_, err := domain.NewTarget(name, urlNotUnique, configUnique, uid)
		testutil.Equals(t, domain.ErrUrlAlreadyTaken, err)
	})

	t.Run("should fail if the config is not unique", func(t *testing.T) {
		_, err := domain.NewTarget(name, urlUnique, configNotUnique, uid)
		testutil.Equals(t, domain.ErrConfigAlreadyTaken, err)
	})

	t.Run("should be instantiable", func(t *testing.T) {
		target, err := domain.NewTarget(name, urlUnique, configUnique, uid)

		testutil.IsNil(t, err)
		testutil.HasNEvents(t, &target, 1)
		evt := testutil.EventIs[domain.TargetCreated](t, &target, 0)

		testutil.NotEquals(t, "", evt.ID)
		testutil.Equals(t, name, evt.Name)
		testutil.Equals(t, targetUrl.String(), evt.Url.String())
		testutil.Equals(t, config, evt.Provider)
		testutil.Equals(t, domain.TargetStatusConfiguring, evt.State.Status())
		testutil.Equals(t, uid, evt.Created.By())
	})

	t.Run("could be renamed and raise the event only if different", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))

		err := target.Rename("new-name")

		testutil.IsNil(t, err)
		evt := testutil.EventIs[domain.TargetRenamed](t, &target, 1)
		testutil.Equals(t, "new-name", evt.Name)

		testutil.IsNil(t, target.Rename("new-name"))
		testutil.HasNEvents(t, &target, 2)
	})

	t.Run("could not be renamed if delete requested", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)
		testutil.IsNil(t, target.RequestCleanup(false, uid))

		testutil.ErrorIs(t, domain.ErrTargetCleanupRequested, target.Rename("new-name"))
	})

	t.Run("could have its domain changed if available and raise the event only if different", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		newUrl := must.Panic(domain.UrlFrom("http://new-url.com"))
		err := target.HasUrl(domain.NewTargetUrlRequirement(newUrl, false))

		testutil.ErrorIs(t, domain.ErrUrlAlreadyTaken, err)

		err = target.HasUrl(domain.NewTargetUrlRequirement(newUrl, true))

		testutil.IsNil(t, err)
		evt := testutil.EventIs[domain.TargetUrlChanged](t, &target, 1)
		testutil.Equals(t, newUrl.String(), evt.Url.String())

		evtTargetChanged := testutil.EventIs[domain.TargetStateChanged](t, &target, 2)
		testutil.Equals(t, domain.TargetStatusConfiguring, evtTargetChanged.State.Status())

		testutil.IsNil(t, target.HasUrl(domain.NewTargetUrlRequirement(newUrl, true)))
		testutil.HasNEvents(t, &target, 3)
	})

	t.Run("could not have its domain changed if delete requested", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)

		newUrl := domain.NewTargetUrlRequirement(must.Panic(domain.UrlFrom("http://new-url.com")), true)

		testutil.IsNil(t, target.RequestCleanup(false, uid))
		testutil.ErrorIs(t, domain.ErrTargetCleanupRequested, target.HasUrl(newUrl))
	})

	t.Run("should forbid a provider change if the fingerprint has changed", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name,
			domain.NewTargetUrlRequirement(must.Panic(domain.UrlFrom("http://docker.localhost")), true), configUnique, uid))

		err := target.HasProvider(domain.NewProviderConfigRequirement(dummyProviderConfig{data: "new-config", fingerprint: "new-fingerprint"}, true))

		testutil.ErrorIs(t, domain.ErrTargetProviderUpdateNotPermitted, err)
	})

	t.Run("could have its provider changed if available and raise the event only if different", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name,
			domain.NewTargetUrlRequirement(must.Panic(domain.UrlFrom("http://docker.localhost")), true),
			configUnique, uid))
		newConfig := dummyProviderConfig{data: "new-config"}

		err := target.HasProvider(domain.NewProviderConfigRequirement(newConfig, false))

		testutil.ErrorIs(t, domain.ErrConfigAlreadyTaken, err)

		err = target.HasProvider(domain.NewProviderConfigRequirement(newConfig, true))

		testutil.IsNil(t, err)
		evt := testutil.EventIs[domain.TargetProviderChanged](t, &target, 1)
		testutil.IsTrue(t, newConfig == evt.Provider)

		evtTargetChanged := testutil.EventIs[domain.TargetStateChanged](t, &target, 2)
		testutil.Equals(t, domain.TargetStatusConfiguring, evtTargetChanged.State.Status())

		testutil.IsNil(t, target.HasProvider(domain.NewProviderConfigRequirement(newConfig, true)))
		testutil.HasNEvents(t, &target, 3)
	})

	t.Run("could not have its provider changed if delete requested", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)

		testutil.IsNil(t, target.RequestCleanup(false, uid))
		testutil.ErrorIs(t, domain.ErrTargetCleanupRequested, target.HasProvider(configUnique))
	})

	t.Run("should raise the TargetStateChanged only once when updating both the domain and the config", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		newUrl := domain.NewTargetUrlRequirement(must.Panic(domain.UrlFrom("http://new-url.com")), true)
		newConfig := domain.NewProviderConfigRequirement(dummyProviderConfig{data: "new-config"}, true)

		testutil.IsNil(t, target.HasUrl(newUrl))
		testutil.IsNil(t, target.HasProvider(newConfig))

		testutil.HasNEvents(t, &target, 4)
		evt := testutil.EventIs[domain.TargetStateChanged](t, &target, 3)
		testutil.Equals(t, domain.TargetStatusConfiguring, evt.State.Status())
	})

	t.Run("could be marked as configured and raise the appropriate event", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))

		target.Configured(target.CurrentVersion().Add(-1*time.Hour), nil)

		testutil.HasNEvents(t, &target, 1)
		testutil.EventIs[domain.TargetCreated](t, &target, 0)

		target.Configured(target.CurrentVersion(), nil)
		target.Configured(target.CurrentVersion(), nil) // Should not raise a new event

		testutil.HasNEvents(t, &target, 2)
		changed := testutil.EventIs[domain.TargetStateChanged](t, &target, 1)
		testutil.Equals(t, domain.TargetStatusReady, changed.State.Status())
	})

	t.Run("could expose its availability based on its internal state", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))

		// Configuring
		err := target.CheckAvailability()

		testutil.ErrorIs(t, domain.ErrTargetConfigurationInProgress, err)

		// Configuration failed
		target.Configured(target.CurrentVersion(), errors.New("configuration failed"))

		err = target.CheckAvailability()

		testutil.ErrorIs(t, domain.ErrTargetConfigurationFailed, err)

		// Configuration success
		target.Reconfigure()

		target.Configured(target.CurrentVersion(), nil)

		err = target.CheckAvailability()

		testutil.IsNil(t, err)

		// Delete requested
		target.RequestCleanup(false, uid)

		err = target.CheckAvailability()

		testutil.ErrorIs(t, domain.ErrTargetCleanupRequested, err)
	})

	t.Run("could not be reconfigured if cleanup requested", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)
		testutil.IsNil(t, target.RequestCleanup(false, uid))

		testutil.ErrorIs(t, domain.ErrTargetCleanupRequested, target.Reconfigure())
	})

	t.Run("could not be reconfigured if configuring", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))

		testutil.ErrorIs(t, domain.ErrTargetConfigurationInProgress, target.Reconfigure())
	})

	t.Run("should not be removed if still used by an app", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)

		testutil.ErrorIs(t, domain.ErrTargetInUse, target.RequestCleanup(true, uid))
	})

	t.Run("should not be removed if configuring", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))

		testutil.ErrorIs(t, domain.ErrTargetConfigurationInProgress, target.RequestCleanup(false, uid))
	})

	t.Run("could be removed if no app is using it", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)

		err := target.RequestCleanup(false, uid)
		testutil.IsNil(t, err)

		testutil.IsNil(t, err)
		testutil.HasNEvents(t, &target, 3)
		evt := testutil.EventIs[domain.TargetCleanupRequested](t, &target, 2)
		testutil.Equals(t, target.ID(), evt.ID)
	})

	t.Run("should not raise an event is the target is already marked has deleting", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)

		testutil.IsNil(t, target.RequestCleanup(false, uid))
		testutil.IsNil(t, target.RequestCleanup(false, uid))

		testutil.HasNEvents(t, &target, 3)
	})

	t.Run("should returns an err if trying to cleanup a target while configuring", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))

		_, err := target.CleanupStrategy(false)

		testutil.ErrorIs(t, domain.ErrTargetConfigurationInProgress, err)
	})

	t.Run("should returns an err if trying to cleanup a target while deployments are still running", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)

		_, err := target.CleanupStrategy(true)

		testutil.ErrorIs(t, domain.ErrRunningOrPendingDeployments, err)
	})

	t.Run("should returns the skip cleanup strategy if the configuration has failed and the target could not be updated anymore", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)
		target.Reconfigure()
		target.Configured(target.CurrentVersion(), errors.New("configuration failed"))
		target.RequestCleanup(false, uid)

		s, err := target.CleanupStrategy(false)

		testutil.IsNil(t, err)
		testutil.Equals(t, domain.CleanupStrategySkip, s)
	})

	t.Run("should returns the skip cleanup strategy if the configuration has failed and has never been reachable", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), errors.New("configuration failed"))

		s, err := target.CleanupStrategy(false)

		testutil.IsNil(t, err)
		testutil.Equals(t, domain.CleanupStrategySkip, s)
	})

	t.Run("should returns an err if the configuration has failed but the target is still updatable", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)
		target.Reconfigure()
		target.Configured(target.CurrentVersion(), errors.New("configuration failed"))

		_, err := target.CleanupStrategy(false)

		testutil.ErrorIs(t, domain.ErrTargetConfigurationFailed, err)
	})

	t.Run("should returns the default strategy if the target is correctly configured", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)

		s, err := target.CleanupStrategy(false)

		testutil.IsNil(t, err)
		testutil.Equals(t, domain.CleanupStrategyDefault, s)
	})

	t.Run("returns an err if trying to cleanup an app while configuring", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))

		_, err := target.AppCleanupStrategy(false, true)

		testutil.ErrorIs(t, domain.ErrTargetConfigurationInProgress, err)
	})

	t.Run("returns a skip strategy when trying to cleanup an app on a deleting target", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)
		testutil.IsNil(t, target.RequestCleanup(false, uid))

		s, err := target.AppCleanupStrategy(false, false)

		testutil.IsNil(t, err)
		testutil.Equals(t, domain.CleanupStrategySkip, s)
	})

	t.Run("returns a skip strategy when trying to cleanup an app when no successful deployment has been made", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))

		s, err := target.AppCleanupStrategy(false, false)

		testutil.IsNil(t, err)
		testutil.Equals(t, domain.CleanupStrategySkip, s)
	})

	t.Run("returns an error when trying to cleanup an app on a failed target", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)
		target.Reconfigure()
		target.Configured(target.CurrentVersion(), errors.New("configuration failed"))

		_, err := target.AppCleanupStrategy(false, true)

		testutil.ErrorIs(t, domain.ErrTargetConfigurationFailed, err)
	})

	t.Run("returns an error when trying to cleanup an app but there are still running or pending deployments", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)

		_, err := target.AppCleanupStrategy(true, false)

		testutil.ErrorIs(t, domain.ErrRunningOrPendingDeployments, err)
	})

	t.Run("returns a default strategy when trying to remove an app and everything is good to process it", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)

		s, err := target.AppCleanupStrategy(false, true)

		testutil.IsNil(t, err)
		testutil.Equals(t, domain.CleanupStrategyDefault, s)
	})

	t.Run("should not be removed if no cleanup request has been set", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))

		err := target.Delete(true)

		testutil.ErrorIs(t, domain.ErrTargetCleanupNeeded, err)
	})

	t.Run("should not be removed if target resources have not been cleaned up", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)
		testutil.IsNil(t, target.RequestCleanup(false, uid)) // No application is using it

		err := target.Delete(false)

		testutil.ErrorIs(t, domain.ErrTargetCleanupNeeded, err)
	})

	t.Run("could be removed if resources have been cleaned up", func(t *testing.T) {
		target := must.Panic(domain.NewTarget(name, urlUnique, configUnique, uid))
		target.Configured(target.CurrentVersion(), nil)
		testutil.IsNil(t, target.RequestCleanup(false, uid))

		err := target.Delete(true)

		testutil.IsNil(t, err)
		testutil.EventIs[domain.TargetDeleted](t, &target, 3)
	})
}

type dummyProviderConfig struct {
	data        string
	fingerprint string
}

func (d dummyProviderConfig) Kind() string        { return "dummy" }
func (d dummyProviderConfig) Fingerprint() string { return d.fingerprint }
func (d dummyProviderConfig) String() string      { return d.fingerprint }

func (d dummyProviderConfig) Equals(other domain.ProviderConfig) bool {
	return d == other
}
