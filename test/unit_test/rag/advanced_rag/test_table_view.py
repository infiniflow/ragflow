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
from rag.advanced_rag.harness.tools.table_view import render_tables, table_view_or_raw

INFOBOX = "<table><tr><td>Born</td><td>1912</td></tr><tr><td>Died</td><td>1954</td></tr><tr><td>Children</td><td>3</td></tr></table>"
RESULTS = "<table><tr><th>Rank</th><th>Name</th><th>Time</th></tr><tr><td>1</td><td>Ada</td><td>9.9</td></tr><tr><td>2</td><td>Alan</td><td>10.1</td></tr></table>"


def test_text_around_a_table_is_kept():
    text = f"The mathematician was born in London.\n{INFOBOX}\nHe died in Wilmslow."
    view = render_tables(text)
    assert "The mathematician was born in London." in view
    assert "He died in Wilmslow." in view
    assert "Children: 3" in view
    assert "<table" not in view


def test_every_table_is_rendered_in_place():
    view = render_tables(f"Infobox:\n{INFOBOX}\nResults:\n{RESULTS}")
    assert view.index("Infobox:") < view.index("Born: 1912") < view.index("Results:") < view.index("| Rank | Name | Time |")
    assert "| 2 | Alan | 10.1 |" in view


def test_table_only_text_renders_the_table():
    assert render_tables(INFOBOX) == "Born: 1912\nDied: 1954\nChildren: 3"


def test_text_without_a_renderable_table_is_returned_raw():
    text = "Before <table><tr><td></td></tr></table> after"
    assert render_tables(text) is None
    assert table_view_or_raw(text) == text


def test_a_table_that_does_not_render_stays_in_the_text():
    empty = "<table><tr><td></td></tr></table>"
    assert render_tables(f"Intro {empty} middle\n{INFOBOX}") == f"Intro {empty} middle\n\nBorn: 1912\nDied: 1954\nChildren: 3"


def test_a_table_tag_inside_a_comment_does_not_end_the_table():
    table = INFOBOX.replace("<tr>", "<!-- </table> --><tr>", 1)
    assert render_tables(f"Intro\n{table}\nOutro") == "Intro\n\nBorn: 1912\nDied: 1954\nChildren: 3\n\nOutro"
