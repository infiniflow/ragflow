---
sidebar_position: 2
title: Basic Information Configuration
sidebar_label: Basic Information Configuration
slug: /knowledge_compilation/basic_information_configuration
sidebar_custom_props: {
  categoryIcon: LucideWandSparkles
}
---

# Basic Information Configuration

When creating or editing a knowledge compilation template, you need to complete the basic information configuration first. Basic information determines the template name, the model used, and the basic compilation method. All knowledge compilation template types include these configuration items.

## Template Name

Sets the name of the knowledge compilation template so different templates can be identified during later configuration and use. It is recommended to name the template based on its actual purpose so that the name clearly reflects the usage scenario.

## Template Description

The template description explains the function, applicable scenarios, and main processing content of the current template, making later viewing and management easier.

## Default Extraction Model

The default extraction model specifies the model used during knowledge compilation. The system uses this model to understand and analyze document content, and completes information extraction and structured generation according to the rules defined in the template.

Select an appropriate model based on actual business requirements and model capabilities. You can refer to the related description in the "Template Selection Recommendations" section.

## Template

Selects the knowledge artifact type to generate through knowledge compilation. The following templates are currently supported:

- Graph
- Tree
- PageIndex
- MindMap
- Timeline
- Wiki

Different templates correspond to different knowledge organization methods and configuration items. After selecting a template, you can continue configuring the parameters for that template. The template is used to select the template type used by the knowledge compilation task.

## Global Rules

Sets the requirements that the current knowledge compilation template must follow uniformly during execution.

The content controlled by global rules differs between templates. For example, Graph can use global rules to constrain entity and relationship extraction, while Wiki can use global rules to control content organization and generation requirements. For specific configuration methods, refer to the corresponding template chapter.

## Re-Split Parser Output

Controls whether Compiler reorganizes and splits Parser output before executing knowledge compilation.

After this option is enabled, Compiler reorganizes Parser output based on the processing requirements of the current knowledge compilation template before executing subsequent knowledge compilation. If disabled, compilation is performed directly based on Parser output.

This setting only affects the knowledge compilation process and does not replace Chunker in the Ingestion Pipeline.

Whether this feature is enabled must be determined when configuring the knowledge compilation template. After the template is saved, the setting takes effect when the template is used for knowledge compilation.

![Re-Split Parser Output](https://raw.githubusercontent.com/infiniflow/ragflow-docs/78dcfd707366b45934720c7abe480897f31ecbe7/images/basic-info-config-rechunk-parser-output.jpg)
