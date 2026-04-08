package quality

import (
	"gostream/internal/config"
	"gostream/internal/catalog/tmdb"
)

// TMDBEndpointGroupFromConfig converts config.TMDBEndpointGroup to tmdb.EndpointConfig.
func TMDBEndpointGroupFromConfig(cfg config.TMDBEndpointGroup) tmdb.EndpointConfig {
	var endpoints []tmdb.Endpoint
	for _, ep := range cfg.Endpoints {
		endpoints = append(endpoints, tmdb.Endpoint{
			Name:         ep.Name,
			Enabled:      ep.Enabled,
			EndpointType: ep.EndpointType,
			Language:     ep.Language,
			SortBy:       ep.SortBy,
			Pages:        ep.Pages,
			VoteAverageGte:       ep.VoteAverageGte,
			VoteCountGte:         ep.VoteCountGte,
			WithGenres:           ep.WithGenres,
			WithoutGenres:        ep.WithoutGenres,
			WithKeywords:         ep.WithKeywords,
			WithoutKeywords:      ep.WithoutKeywords,
			WithOriginalLanguage: ep.WithOriginalLanguage,
			WithOriginCountry:    ep.WithOriginCountry,
			WithRuntimeGte:       ep.WithRuntimeGte,
			WithRuntimeLte:       ep.WithRuntimeLte,
			WatchRegion:          ep.WatchRegion,
			IncludeAdult:         ep.IncludeAdult,
			PrimaryReleaseDateGte: ep.PrimaryReleaseDateGte,
			PrimaryReleaseDateLte: ep.PrimaryReleaseDateLte,
			PrimaryReleaseYear:   ep.PrimaryReleaseYear,
			WithReleaseType:      ep.WithReleaseType,
			Region:               ep.Region,
			IncludeVideo:         ep.IncludeVideo,
			FirstAirDateGte:      ep.FirstAirDateGte,
			FirstAirDateLte:      ep.FirstAirDateLte,
			FirstAirDateYear:     ep.FirstAirDateYear,
			WithStatus:           ep.WithStatus,
			WithType:             ep.WithType,
			WithNetworks:         ep.WithNetworks,
			IncludeNullFirstAirDates: ep.IncludeNullFirstAirDates,
			EndpointURL:          ep.EndpointURL,
			TimeWindow:           ep.TimeWindow,
		})
	}
	return tmdb.EndpointConfig{Endpoints: endpoints}
}
