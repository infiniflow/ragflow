import re

with open('agent/tools/retrieval.py', 'r') as f:
    content = f.read()

content = content.replace('''<<<<<<< HEAD
        self.meta_data_filter={}
        self.temporal_retrieval={}
=======
        self.meta_data_filter = {}
>>>>>>> origin/infinitiflow-main''', '''        self.meta_data_filter = {}
        self.temporal_retrieval = {}''')

content = content.replace('''<<<<<<< HEAD
        chat_mdl = None
        if self._param.meta_data_filter.get("method") in ["auto", "semi_auto"]:
            tenant_id = self._canvas.get_tenant_id()
            chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
            chat_mdl = LLMBundle(tenant_id, chat_model_config)
        raw_query = query
=======
        if self._param.meta_data_filter != {}:
            # Defer the (potentially expensive) metadata table load — manual
            # filters served by ES push-down never need it. The loader is
            # invoked at most once per request by ``apply_meta_data_filter``.
            def _load_metas() -> dict:
                return DocMetadataService.get_flatted_meta_by_kbs(kb_ids)

            def _resolve_manual_filter(flt: dict) -> dict:
                # Return a new dict instead of mutating `flt` in place. The
                # caller passes filters straight out of self._param.meta_data_filter,
                # so mutating them would replace the variable reference with its
                # resolved value and every subsequent invocation (e.g. inside an
                # Iteration component) would reuse that stale value.
                pat = re.compile(self.variable_ref_patt)
                s = flt.get("value", "")
                out_parts = []
                last = 0

                for m in pat.finditer(s):
                    out_parts.append(s[last : m.start()])
                    key = m.group(1)
                    v = self._canvas.get_variable_value(key)
                    if v is None:
                        rep = ""
                    elif isinstance(v, partial):
                        buf = []
                        for chunk in v():
                            buf.append(chunk)
                        rep = "".join(buf)
                    elif isinstance(v, str):
                        rep = v
                    else:
                        rep = json.dumps(v, ensure_ascii=False)

                    out_parts.append(rep)
                    last = m.end()

                out_parts.append(s[last:])
                resolved = dict(flt)
                resolved["value"] = "".join(out_parts)
                return resolved

            chat_mdl = None
            if self._param.meta_data_filter.get("method") in ["auto", "semi_auto"]:
                tenant_id = self._canvas.get_tenant_id()
                chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
                chat_mdl = LLMBundle(tenant_id, chat_model_config)

            doc_ids = await apply_meta_data_filter(
                self._param.meta_data_filter,
                None,
                query,
                chat_mdl,
                doc_ids,
                _resolve_manual_filter if self._param.meta_data_filter.get("method") == "manual" else None,
                kb_ids=kb_ids,
                metas_loader=_load_metas,
            )
>>>>>>> origin/infinitiflow-main''', '''        def _load_metas() -> dict:
            return DocMetadataService.get_flatted_meta_by_kbs(kb_ids)

        def _resolve_manual_filter(flt: dict) -> dict:
            # Return a new dict instead of mutating `flt` in place. The
            # caller passes filters straight out of self._param.meta_data_filter,
            # so mutating them would replace the variable reference with its
            # resolved value and every subsequent invocation (e.g. inside an
            # Iteration component) would reuse that stale value.
            pat = re.compile(self.variable_ref_patt)
            s = flt.get("value", "")
            out_parts = []
            last = 0

            for m in pat.finditer(s):
                out_parts.append(s[last : m.start()])
                key = m.group(1)
                v = self._canvas.get_variable_value(key)
                if v is None:
                    rep = ""
                elif isinstance(v, partial):
                    buf = []
                    for chunk in v():
                        buf.append(chunk)
                    rep = "".join(buf)
                elif isinstance(v, str):
                    rep = v
                else:
                    rep = json.dumps(v, ensure_ascii=False)

                out_parts.append(rep)
                last = m.end()

            out_parts.append(s[last:])
            resolved = dict(flt)
            resolved["value"] = "".join(out_parts)
            return resolved

        chat_mdl = None
        method = self._param.meta_data_filter.get("method")
        temporal_method = getattr(self._param, "temporal_retrieval", {}).get("method") if getattr(self._param, "temporal_retrieval", {}) else None
        
        if method in ["auto", "semi_auto"] or temporal_method in ["auto", "semi_auto"]:
            tenant_id = self._canvas.get_tenant_id()
            chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
            chat_mdl = LLMBundle(tenant_id, chat_model_config)

        raw_query = query

        if self._param.meta_data_filter != {}:
            doc_ids = await apply_meta_data_filter(
                self._param.meta_data_filter,
                None,
                query,
                chat_mdl,
                doc_ids,
                _resolve_manual_filter if self._param.meta_data_filter.get("method") == "manual" else None,
                kb_ids=kb_ids,
                metas_loader=_load_metas,
            )''')

with open('agent/tools/retrieval.py', 'w') as f:
    f.write(content)
