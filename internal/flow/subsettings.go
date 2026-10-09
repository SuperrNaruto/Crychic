package flow

import "slices"

type subscriptionUpgrade struct{ enabled, full bool }

// SubscriptionConditions are editable settings, never runtime progress facts.
// Include/Exclude stay opaque; chat input is converted from literal keywords.
type SubscriptionConditions struct {
	Quality, Resolution, Effect  string
	Include, Exclude             string
	FilterGroups                 []string
	Sites                        []int
	StartEpisode                 int
	BestVersion, BestVersionFull bool
}

func (s Subscription) conditions() SubscriptionConditions {
	return SubscriptionConditions{
		Quality: s.Quality, Resolution: s.Resolution, Effect: s.Effect,
		Include: s.Include, Exclude: s.Exclude,
		FilterGroups: slices.Clone(s.FilterGroups), Sites: slices.Clone(s.Sites),
		StartEpisode: s.StartEpisode, BestVersion: s.BestVersion, BestVersionFull: s.BestVersionFull,
	}
}

func (s SubscriptionConditions) same(other SubscriptionConditions) bool {
	return s.Quality == other.Quality && s.Resolution == other.Resolution && s.Effect == other.Effect &&
		s.Include == other.Include && s.Exclude == other.Exclude && s.StartEpisode == other.StartEpisode &&
		s.BestVersion == other.BestVersion && s.BestVersionFull == other.BestVersionFull &&
		slices.Equal(s.FilterGroups, other.FilterGroups) && slices.Equal(s.Sites, other.Sites)
}

func (s SubscriptionConditions) changedField(other SubscriptionConditions, field int) bool {
	switch field {
	case editQuality:
		return s.Quality != other.Quality
	case editResolution:
		return s.Resolution != other.Resolution
	case editEffect:
		return s.Effect != other.Effect
	case editGroups:
		return !slices.Equal(s.FilterGroups, other.FilterGroups)
	case editSites:
		return !slices.Equal(s.Sites, other.Sites)
	case editInclude:
		return s.Include != other.Include
	case editExclude:
		return s.Exclude != other.Exclude
	case editStart:
		return s.StartEpisode != other.StartEpisode
	case editUpgrade:
		return subscriptionUpgrade{s.BestVersion, s.BestVersionFull} != (subscriptionUpgrade{other.BestVersion, other.BestVersionFull})
	}
	return false
}

// SubscriptionUpdate contains a base and a draft so the backend can send only
// actual changes. ID is the sole identity field the upstream write accepts.
type SubscriptionUpdate struct {
	ID            int
	Before, After SubscriptionConditions
}

// SubscriptionRuleGroup omits raw custom expressions, usage and private errors.
type SubscriptionRuleGroup struct {
	Name        string
	Kind        Kind // zero applies to either supported media kind
	Category    string
	Fingerprint string // definition and referenced rules, never displayed
	Available   bool
}
