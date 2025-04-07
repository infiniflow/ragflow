#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import random
from datetime import datetime


class SimpleFunctionCallServer:
    def __init__(self):
        pass

    def tool_call(self, name: str, arguments: dict):
        if name == "get_current_weather":
            return self.get_current_weather(arguments.get("location", ""))
        elif name == "get_current_time":
            return self.get_current_time()

    def get_current_weather(self, location: str) -> str:
        """
        Get current weather of location.

        Args:
            location: location string (e.g. Shanghai)
        """
        weather_conditions = ["sunny", "cloudy", "rainy"]
        random_weather = random.choice(weather_conditions)
        return f"The weather of {location} is {random_weather}."

    def get_current_time(self) -> str:
        return datetime.now().strftime("%Y-%m-%d %H:%M:%S")


if __name__ == "__main__":
    pass
