#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

from pydantic import BaseModel


def get_field_value(model: BaseModel, name_or_alias: str):
    """
    Retrieve the value of a field from a Pydantic BaseModel.

    This function first attempts to find a field by its alias. 
    If no field matches the alias, it then tries to match the internal attribute name.
    Returns the value of the first matching field.

    Args:
        model (BaseModel): The Pydantic model instance to query.
        name_or_alias (str): The alias or attribute name of the field.

    Returns:
        Any: The value of the field.

    Raises:
        AttributeError: If no field with the given alias or name is found.
    """

    # Check alias first
    for name, field in model.model_fields.items():
        if field.alias == name_or_alias:
            return getattr(model, name)

    # Fallback: check internal field name
    if name_or_alias in model.model_fields:
        return getattr(model, name_or_alias)

    raise AttributeError(f"No field with alias or name '{name_or_alias}'")
