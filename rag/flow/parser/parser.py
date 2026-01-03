            parser_model_name = resolve_mineru_llm_name()
            if not parser_model_name:
                raise RuntimeError("MinerU model not configured. Please add MinerU in Model Providers or set MINERU_* env.")

            tenant_id = self._canvas._tenant_id
            ocr_model = LLMBundle(tenant_id, LLMType.OCR, llm_name=parser_model_name, lang=conf.get("lang", "Chinese"))
            pdf_parser = ocr_model.mdl

            # Extract MinerU-specific configuration
            # Note: All configuration is passed via parser_config to ensure consistency
            # The default for mineru_parse_method is "auto" which lets MinerU automatically
            # determine the best parsing method (txt or ocr) based on the PDF content
            parser_config = {
                'mineru_parse_method': conf.get("mineru_parse_method", "auto"),
                'mineru_lang': conf.get("lang", "Chinese"),
                'mineru_formula_enable': conf.get("mineru_formula_enable", True),
                'mineru_table_enable': conf.get("mineru_table_enable", True),
                'mineru_batch_size': conf.get("mineru_batch_size", 30),
                'mineru_start_page': conf.get("mineru_start_page"),
                'mineru_end_page': conf.get("mineru_end_page"),
            }

            lines, _ = pdf_parser.parse_pdf(
                filepath=name,
                binary=blob,
                callback=self.callback,
                parser_config=parser_config,
            )
            bboxes = []
            for t, poss in lines:
                box = {
                    "image": pdf_parser.crop(poss, 1),
                    "positions": [[pos[0][-1], *pos[1:]] for pos in pdf_parser.extract_positions(poss)],
                    "text": t,
                }
                bboxes.append(box)
