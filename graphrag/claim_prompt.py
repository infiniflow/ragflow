# Copyright (c) 2024 Microsoft Corporation.
# Licensed under the MIT License
"""
Reference:
 - [graphrag](https://github.com/microsoft/graphrag)
"""

CLAIM_EXTRACTION_PROMPT = """
################
-Target activity-
################
You are an intelligent assistant that helps a human analyst to analyze claims against certain entities presented in a text document.

################
-Goal-
################
Given a text document that is potentially relevant to this activity, an entity specification, and a claim description, extract all entities that match the entity specification and all claims against those entities.

################
-Steps-
################
 - 1. Extract all named entities that match the predefined entity specification. Entity specification can either be a list of entity names or a list of entity types.
 - 2. For each entity identified in step 1, extract all claims associated with the entity. Claims need to match the specified claim description, and the entity should be the subject of the claim.
    For each claim, extract the following information:
    - Subject: name of the entity that is subject of the claim, capitalized. The subject entity is one that committed the action described in the claim. Subject needs to be one of the named entities identified in step 1.
    - Object: name of the entity that is object of the claim, capitalized. The object entity is one that either reports/handles or is affected by the action described in the claim. If object entity is unknown, use **NONE**.
    - Claim Type: overall category of the claim, capitalized. Name it in a way that can be repeated across multiple text inputs, so that similar claims share the same claim type
    - Claim Status: **TRUE**, **FALSE**, or **SUSPECTED**. TRUE means the claim is confirmed, FALSE means the claim is found to be False, SUSPECTED means the claim is not verified.
    - Claim Description: Detailed description explaining the reasoning behind the claim, together with all the related evidence and references.
    - Claim Date: Period (start_date, end_date) when the claim was made. Both start_date and end_date should be in ISO-8601 format. If the claim was made on a single date rather than a date range, set the same date for both start_date and end_date. If date is unknown, return **NONE**.
    - Claim Source Text: List of **all** quotes from the original text that are relevant to the claim.

 - 3. Format each claim as (<subject_entity>{tuple_delimiter}<object_entity>{tuple_delimiter}<claim_type>{tuple_delimiter}<claim_status>{tuple_delimiter}<claim_start_date>{tuple_delimiter}<claim_end_date>{tuple_delimiter}<claim_description>{tuple_delimiter}<claim_source>)
 - 4. Return output in language of the 'Text' as a single list of all the claims identified in steps 1 and 2. Use **{record_delimiter}** as the list delimiter.
 - 5. If there's nothing satisfy the above requirements, just keep output empty.
 - 6. When finished, output {completion_delimiter}

################
-Examples-
################
Example 1:
Entity specification: organization
Claim description: red flags associated with an entity
Text: According to an article on 2022/01/10, Company A was fined for bid rigging while participating in multiple public tenders published by Government Agency B. The company is owned by Person C who was suspected of engaging in corruption activities in 2015.
Output:
(COMPANY A{tuple_delimiter}GOVERNMENT AGENCY B{tuple_delimiter}ANTI-COMPETITIVE PRACTICES{tuple_delimiter}TRUE{tuple_delimiter}2022-01-10T00:00:00{tuple_delimiter}2022-01-10T00:00:00{tuple_delimiter}Company A was found to engage in anti-competitive practices because it was fined for bid rigging in multiple public tenders published by Government Agency B according to an article published on 2022/01/10{tuple_delimiter}According to an article published on 2022/01/10, Company A was fined for bid rigging while participating in multiple public tenders published by Government Agency B.)
{completion_delimiter}

###########################
Example 2:
Entity specification: Company A, Person C
Claim description: red flags associated with an entity
Text: According to an article on 2022/01/10, Company A was fined for bid rigging while participating in multiple public tenders published by Government Agency B. The company is owned by Person C who was suspected of engaging in corruption activities in 2015.
Output:
(COMPANY A{tuple_delimiter}GOVERNMENT AGENCY B{tuple_delimiter}ANTI-COMPETITIVE PRACTICES{tuple_delimiter}TRUE{tuple_delimiter}2022-01-10T00:00:00{tuple_delimiter}2022-01-10T00:00:00{tuple_delimiter}Company A was found to engage in anti-competitive practices because it was fined for bid rigging in multiple public tenders published by Government Agency B according to an article published on 2022/01/10{tuple_delimiter}According to an article published on 2022/01/10, Company A was fined for bid rigging while participating in multiple public tenders published by Government Agency B.)
{record_delimiter}
(PERSON C{tuple_delimiter}NONE{tuple_delimiter}CORRUPTION{tuple_delimiter}SUSPECTED{tuple_delimiter}2015-01-01T00:00:00{tuple_delimiter}2015-12-30T00:00:00{tuple_delimiter}Person C was suspected of engaging in corruption activities in 2015{tuple_delimiter}The company is owned by Person C who was suspected of engaging in corruption activities in 2015)
{completion_delimiter}

################
-Real Data-
################
Use the following input for your answer.
Entity specification: {entity_specs}
Claim description: {claim_description}
Text: {input_text}
Output:"""


CONTINUE_PROMPT = "MANY entities were missed in the last extraction.  Add them below using the same format(see 'Steps', start with the 'Output').\nOutput: "
LOOP_PROMPT = "It appears some entities may have still been missed.  Answer YES {tuple_delimiter} NO if there are still entities that need to be added.\n"