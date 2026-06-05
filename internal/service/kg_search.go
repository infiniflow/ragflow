package service

import "ragflow/internal/service/kg"

// Type aliases for backward compatibility
type KGEntity = kg.KGEntity
type KGRelation = kg.KGRelation
type KGCommunityReport = kg.KGCommunityReport
type NhopEntity = kg.NhopEntity
type Edge = kg.Edge
type EdgeScore = kg.EdgeScore
type ScoredEntity = kg.ScoredEntity
type ScoredRelation = kg.ScoredRelation

// Function aliases for backward compatibility
var (
	SearchKGEntities            = kg.SearchKGEntities
	SearchKGEntitiesByTypes     = kg.SearchKGEntitiesByTypes
	SearchKGRelations            = kg.SearchKGRelations
	SearchKGCommunityReports    = kg.SearchKGCommunityReports
	SearchKGTypeSamples         = kg.SearchKGTypeSamples
	NhopEntityNames             = kg.NhopEntityNames
	ParseKGEntityChunks         = kg.ParseKGEntityChunks
	ParseKGRelationChunks       = kg.ParseKGRelationChunks
	ParseKGCommunityReportChunks = kg.ParseKGCommunityReportChunks
	ParseKGTypeSamplesChunks    = kg.ParseKGTypeSamplesChunks
	BuildKGContent              = kg.BuildKGContent
	FormatEntitiesToCSV         = kg.FormatEntitiesToCSV
	FormatRelationsToCSV        = kg.FormatRelationsToCSV
	FilterChunksByScore         = kg.FilterChunksByScore
	NumTokensFromString         = kg.NumTokensFromString
	AnalyzeNHopPaths            = kg.AnalyzeNHopPaths
	DoubleHitBoost              = kg.DoubleHitBoost
	FuseRelationScores          = kg.FuseRelationScores
	SortAndTrimEntities         = kg.SortAndTrimEntities
	SortAndTrimRelations        = kg.SortAndTrimRelations
)
