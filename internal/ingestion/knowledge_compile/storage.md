# Knowledge compilation storage

`compile_kwd` identifies the producing knowledge compilation kind. New rows use
`graph`, `mind_map`, `page_index`, `tree`, `timeline`, or `wiki`. The existing
session kinds remain `session_graph` and `session_essence`.

`type_kwd` identifies the row's role:

| Compilation | Roles |
| --- | --- |
| Graph, mind map, page index | `entity`, `relation`, `kg_build_meta` |
| Tree | Existing entity/relation roles; root/summary metadata remains in `raptor_kwd` |
| Navigation from tree/page index | `nav_doc`, `nav_cluster` |
| Wiki | `wiki_page`, `wiki_section`, `wiki_entity`, `wiki_relation`, `wiki_contribution_state`, `wiki_map_active`, `wiki_map_extract` |

Wiki page categories such as `concept`, `entity`, and `topic` are stored in
`entity_type_kwd`. Inferred structure extraction types such as `list`, `set`,
and `hypergraph` are retained as `compile_type` inside the JSON string in `extra`.
Neither changes the compilation kind.

New writes omit `compilation_template_kind_kwd` and `page_type_kwd`.
Their engine columns remain available for reading existing data.

Navigation document leaves retain the source compilation kind. Topic clusters
retain the kind of the compilation that created them; clusters can contain
documents compiled by both tree and page index. Navigation selection uses the
row role, so these clusters remain accessible across both kinds. Navigation
rows remain hidden from ordinary retrieval with `available_int=0`.

Elasticsearch and Infinity translate compilation predicates into one query
covering both stored generations. For example, selecting the `wiki_page` role
matches either `(compile_kwd=wiki AND type_kwd=wiki_page)` or the old
`compile_kwd=wiki_page`. Compilation-kind reads prefer canonical `compile_kwd`
values on rows without the retired template-kind stamp. Stamped rows retain
their original template kind, including when their inferred type happens to
equal a canonical kind. Wiki category reads prefer `entity_type_kwd` and
otherwise read `page_type_kwd`. Tenant/dataset and document constraints remain
outside the alternative branches.

Rebuild cleanup selects navigation by role and merged structure rows by dataset
scope. It must not delete all rows sharing a compilation kind, since document
inputs and dataset outputs now use the same canonical value.
