                s = s.knn(
                    m.vector_column_name,
                    k,
                    num_candidates,
                    query_vector=list(m.embedding_data),
                    filter=_build_knn_filter_query(bool_query, vector_similarity_weight),
                    similarity=similarity,
                )