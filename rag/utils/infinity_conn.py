            vector_similarity_weight = _vector_similarity_weight(match_expressions)
            for matchExpr in match_expressions:
                if isinstance(matchExpr, MatchTextExpr):
                    if filter_cond and "filter" not in matchExpr.extra_options:
                        matchExpr.extra_options.update({"filter": filter_cond})
                    matchExpr.fields = [self.convert_matching_field(field) for field in matchExpr.fields]
                    fields = ",".join(matchExpr.fields)
                    filter_fulltext = f"filter_fulltext('{fields}', '{matchExpr.matching_text}')"
                    if filter_cond:
                        filter_fulltext = f"({filter_cond}) AND {filter_fulltext}"
                    minimum_should_match = matchExpr.extra_options.get("minimum_should_match", 0.0)
                    if isinstance(minimum_should_match, float):
                        str_minimum_should_match = format_minimum_should_match_percent(minimum_should_match)
                        matchExpr.extra_options["minimum_should_match"] = str_minimum_should_match

                    # Add rank_feature support
                    if rank_feature and "rank_features" not in matchExpr.extra_options:
                        # Convert rank_feature dict to Infinity's rank_features string format
                        # Format: "field^feature_name^weight,field^feature_name^weight"
                        rank_features_list = []
                        for feature_name, weight in rank_feature.items():
                            # Use TAG_FLD as the field containing rank features
                            rank_features_list.append(f"{TAG_FLD}^{feature_name}^{weight}")
                        if rank_features_list:
                            matchExpr.extra_options["rank_features"] = ",".join(rank_features_list)

                    for k, v in matchExpr.extra_options.items():
                        if not isinstance(v, str):
                            matchExpr.extra_options[k] = str(v)
                    self.logger.debug(f"INFINITY search MatchTextExpr: {json.dumps(matchExpr.__dict__)}")
                elif isinstance(matchExpr, MatchDenseExpr):
                    matchExpr.extra_options.pop("num_candidates", None)
                    dense_filter = _build_dense_filter(filter_cond, filter_fulltext, vector_similarity_weight)
                    if dense_filter and "filter" not in matchExpr.extra_options:
                        matchExpr.extra_options.update({"filter": dense_filter})
                    for k, v in matchExpr.extra_options.items():
                        if not isinstance(v, str):
                            matchExpr.extra_options[k] = str(v)
                    similarity = matchExpr.extra_options.get("similarity")
                    if similarity:
                        matchExpr.extra_options["threshold"] = similarity
                        del matchExpr.extra_options["similarity"]
                    self.logger.debug(f"INFINITY search MatchDenseExpr: {json.dumps(matchExpr.__dict__)}")