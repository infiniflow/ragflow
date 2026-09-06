"""QA-only bootstrap repair; current init_web_data does not seed factories.

New evidence helper extension; canonical catalog data, no production execution.
"""

import json
import os
import psycopg2

assert os.environ["POSTGRES_HOST"] == "postgres"
with open("/ragflow/conf/llm_factories.json") as source:
    catalog = json.load(source)["factory_llm_infos"]
with psycopg2.connect(host="postgres", dbname=os.environ["POSTGRES_DBNAME"], user=os.environ["POSTGRES_USER"], password=os.environ["POSTGRES_PASSWORD"]) as connection:
    with connection.cursor() as cursor:
        cursor.execute("SELECT count(*) FROM llm_factories")
        assert cursor.fetchone()[0] == 0, "Only an empty disposable catalog can be seeded"
        cursor.executemany(
            "INSERT INTO llm_factories(name,logo,tags,rank,status) VALUES (%s,%s,%s,%s,%s)",
            [(row["name"], row.get("logo", ""), row["tags"], int(row.get("rank", 0)), row.get("status", "1")) for row in catalog],
        )
print(f"Seeded {len(catalog)} canonical QA factory records")
