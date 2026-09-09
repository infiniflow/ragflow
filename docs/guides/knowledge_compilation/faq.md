---
sidebar_position: 5
title: FAQ
sidebar_label: FAQ
slug: /knowledge_compilation/faq
sidebar_custom_props: {
  categoryIcon: LucideWandSparkles
}
---

# FAQ

## 1. Why can't I see Wiki in Artifacts after knowledge compilation is complete?

Wiki is generated differently from other knowledge artifacts.

Graph, Tree, PageIndex, MindMap, and Timeline are document-level knowledge artifacts. Their corresponding results can be viewed after knowledge compilation is complete.

Wiki is a knowledge-base-level knowledge artifact. After document knowledge compilation is complete, you still need to go to the Artifacts page of the knowledge base and click generate. The system then generates Wiki based on the compilation results in the current knowledge base.

## 2. Why are no corresponding knowledge artifacts generated after knowledge compilation?

Check the following items in sequence:

- Whether Compiler has been added to the Ingestion Pipeline.
- Whether Compiler has selected the correct knowledge compilation template.
- Whether the knowledge compilation task executed successfully.
- Whether the knowledge compilation template has been correctly configured and saved.
- Whether the default extraction model can be used normally.

If the task execution fails, use the task execution logs to further check the specific cause.

## 3. Why is the generated knowledge artifact incomplete or inconsistent with expectations?

The generation result of a knowledge artifact is affected by factors such as the original document content, selected template, default extraction model, and template configuration.

It is recommended to first check whether the parsing result of the original document is complete. Then adjust the global rules and the configuration parameters of the corresponding template based on the generated result, and execute knowledge compilation again.

## 4. Will already generated knowledge artifacts update automatically after I modify a knowledge compilation template?

No. After modifying template configuration, you need to use the updated template to execute knowledge compilation again before the new configuration is applied to the generated result.

## 5. Can I use different knowledge compilation templates for the same document?

Yes. You can select different knowledge compilation templates based on actual usage scenarios to generate different types of knowledge artifacts, such as Graph, Tree, PageIndex, MindMap, or Timeline.

Different templates have different knowledge organization methods and applicable scenarios. For details, refer to the template selection recommendations.

## 6. What is the difference between "Re-Split Parser Output" and Chunker?

Re-Split Parser Output controls whether Compiler reorganizes and splits Parser output based on the processing requirements of the current template before knowledge compilation.

Chunker is used for document chunk processing in the Ingestion Pipeline.

They act at different processing stages. Re-Split Parser Output does not replace Chunker.

## 7. Why do results differ when the same document uses different models?

During knowledge compilation, the model is responsible for tasks such as entity extraction, content understanding, and structure generation.

Different models may differ in understanding capability, context length, and generation capability, so the final knowledge artifacts may also differ. Select an appropriate model based on document type, content complexity, and the knowledge compilation template used.

## 8. What should I adjust first when the knowledge compilation result is unsatisfactory?

It is recommended to first confirm whether the original document parsing result is correct.

If the parsing result is normal, check and adjust the default extraction model, global rules, and specific configuration parameters of the current template in sequence. After adjustment, execute knowledge compilation again and compare whether the new knowledge artifact meets expectations.
