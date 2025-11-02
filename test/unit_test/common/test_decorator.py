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

from common.decorator import singleton


# Test class for demonstration
@singleton
class TestClass:
    def __init__(self):
        self.counter = 0

    def increment(self):
        self.counter += 1
        return self.counter


# Test cases
class TestSingleton:

    def test_state_persistence(self):
        """Test that instance state persists across multiple calls"""
        instance1 = TestClass()
        instance1.increment()
        instance1.increment()

        instance2 = TestClass()
        assert instance2.counter == 2  # State should persist

    def test_multiple_calls_consistency(self):
        """Test consistency across multiple calls"""
        instances = [TestClass() for _ in range(5)]

        # All references should point to the same object
        first_instance = instances[0]
        for instance in instances:
            assert instance is first_instance

    def test_instance_methods_work(self):
        """Test that instance methods work correctly"""
        instance = TestClass()

        # Test method calls
        result1 = instance.increment()
        result2 = instance.increment()

        assert result1 == 3
        assert result2 == 4
        assert instance.counter == 4


# Test decorator itself
def test_singleton_decorator_returns_callable():
    """Test that the decorator returns a callable"""

    class PlainClass:
        pass

    decorated_class = singleton(PlainClass)

    # Should return a function
    assert callable(decorated_class)

    # Calling should return an instance of PlainClass
    instance = decorated_class()
    assert isinstance(instance, PlainClass)
