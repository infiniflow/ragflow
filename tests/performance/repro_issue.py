import time
import uuid
import logging
from peewee import *
from peewee import Table

# Setup DB
db = SqliteDatabase(':memory:')

class QueryCounter:
    def __init__(self, db):
        self.db = db
        self.count = 0
        self.original_execute = db.execute_sql

    def __enter__(self):
        self.count = 0
        def side_effect(*args, **kwargs):
            self.count += 1
            return self.original_execute(*args, **kwargs)
        self.db.execute_sql = side_effect
        return self

    def __exit__(self, *args):
        self.db.execute_sql = self.original_execute

class File(Model):
    id = CharField(primary_key=True, max_length=32)
    parent_id = CharField(index=True, max_length=32)
    name = CharField(max_length=255)
    type = CharField(max_length=32)
    size = IntegerField(default=0)

    class Meta:
        database = db

def get_uuid():
    return str(uuid.uuid4().hex)

def setup_db():
    if not db.is_closed():
        db.close()
    db.connect()
    db.create_tables([File])

def create_data(num_roots=5, depth=2, fanout=3):
    """
    Creates a folder structure.
    Root -> 5 folders (R1..R5)
    R1 -> 3 folders, 3 files
    ...
    """
    root_id = get_uuid()
    # Root folder itself
    File.create(id=root_id, parent_id=root_id, name="root", type="folder", size=0)

    def create_recursive(parent_id, current_depth):
        if current_depth >= depth:
            return 0

        my_size = 0
        for i in range(fanout):
            # Create subfolder
            fid = get_uuid()
            File.create(id=fid, parent_id=parent_id, name=f"f_{current_depth}_{i}", type="folder", size=0)

            # Recurse
            child_size = create_recursive(fid, current_depth + 1)
            my_size += child_size

            # Create file
            fid_file = get_uuid()
            f_size = 100
            File.create(id=fid_file, parent_id=parent_id, name=f"file_{current_depth}_{i}", type="file", size=f_size)
            my_size += f_size

        return my_size

    # Create direct children of root
    for i in range(num_roots):
        fid = get_uuid()
        File.create(id=fid, parent_id=root_id, name=f"RootChild_{i}", type="folder", size=0)
        create_recursive(fid, 1) # depth 1

    return root_id

def get_folder_size_legacy(folder_id):
    size = 0
    def dfs(parent_id):
        nonlocal size
        # Query children
        query = File.select(File.id, File.size, File.type).where((File.parent_id == parent_id) & (File.id != parent_id))
        for f in query:
            size += f.size
            if f.type == 'folder':
                dfs(f.id)
    dfs(folder_id)
    return size

def get_by_pf_id_legacy(pf_id):
    # Mimic get_by_pf_id loop
    files = list(File.select().where((File.parent_id == pf_id) & (File.id != pf_id)).dicts())

    for file in files:
        if file["type"] == 'folder':
            file["size"] = get_folder_size_legacy(file["id"])
            # check children
            children = list(File.select().where((File.parent_id == file["id"]) & (File.id != file["id"])).dicts())
            file["has_child_folder"] = any(c["type"] == 'folder' for c in children)
        else:
            # mimic get_kb_id_by_file_id (simulated cost: 1 query)
            File.select().where(File.id == file["id"]).count()

    return files

# --- Optimized Implementation ---

def get_folder_sizes_optimized(folder_ids):
    if not folder_ids:
        return {}

    cte_ref = Table('folder_tree')
    anchor = File.select(File.id.alias('root_id'), File.id).where(File.id << folder_ids)

    FA = File.alias()
    recursive = (FA
                 .select(cte_ref.c.root_id, FA.id)
                 .join(cte_ref, on=(FA.parent_id == cte_ref.c.id))
                 .where(FA.id != FA.parent_id))

    cte = anchor.union_all(recursive).cte('folder_tree', recursive=True, columns=('root_id', 'id'))

    query = (File
             .select(cte.c.root_id, fn.SUM(File.size).alias('total_size'))
             .join(cte, on=(File.id == cte.c.id))
             .with_cte(cte)
             .group_by(cte.c.root_id))

    return {row['root_id']: row['total_size'] for row in query.dicts()}

def get_has_child_folders_optimized(folder_ids):
    if not folder_ids:
        return set()
    query = (File
             .select(File.parent_id)
             .where(
                 (File.parent_id << folder_ids) &
                 (File.type == 'folder') &
                 (File.id != File.parent_id)
             )
             .group_by(File.parent_id))
    return set(row.parent_id for row in query)

def get_by_pf_id_optimized(pf_id):
    files = list(File.select().where((File.parent_id == pf_id) & (File.id != pf_id)).dicts())

    folder_ids = [f["id"] for f in files if f["type"] == 'folder']
    file_ids = [f["id"] for f in files if f["type"] != 'folder']

    folder_sizes = get_folder_sizes_optimized(folder_ids) if folder_ids else {}
    has_children_map = get_has_child_folders_optimized(folder_ids) if folder_ids else set()

    # Simulate KB info batch fetch (1 query)
    if file_ids:
        File.select().where(File.id << file_ids).count()

    for file in files:
        if file["type"] == 'folder':
            file["size"] = folder_sizes.get(file["id"], 0)
            file["has_child_folder"] = file["id"] in has_children_map
        else:
            pass

    return files


if __name__ == "__main__":
    setup_db()
    print("Generating data...")
    root_id = create_data(num_roots=10, depth=3, fanout=3)

    print(f"Data generated. Root ID: {root_id}")
    print(f"Total files in DB: {File.select().count()}")

    # Run Legacy Benchmark
    print("\n--- Legacy Benchmark ---")
    start_time = time.time()
    with QueryCounter(db) as qc:
        legacy_results = get_by_pf_id_legacy(root_id)
    end_time = time.time()
    legacy_queries = qc.count

    print(f"Time taken: {end_time - start_time:.4f}s")
    print(f"Total Queries: {qc.count}")

    # Run Optimized Benchmark
    print("\n--- Optimized Benchmark ---")
    start_time = time.time()
    with QueryCounter(db) as qc:
        optimized_results = get_by_pf_id_optimized(root_id)
    end_time = time.time()

    print(f"Time taken: {end_time - start_time:.4f}s")
    print(f"Total Queries: {qc.count}")

    # Validation
    print("\n--- Validation ---")
    print(f"Legacy result count: {len(legacy_results)}")
    print(f"Optimized result count: {len(optimized_results)}")

    match = True
    for l, o in zip(legacy_results, optimized_results):
        if l["id"] != o["id"]:
            print(f"ID mismatch: {l['id']} vs {o['id']}")
            match = False
            break
        if l.get("size") != o.get("size"):
            # Size might be missing if not folder
            if l.get("type") == "folder":
                print(f"Size mismatch for {l['id']}: {l['size']} vs {o['size']}")
                match = False

    if match:
        print("Results Match! ✅")
    else:
        print("Results Mismatch! ❌")

    print(f"\nImprovement: {legacy_queries} -> {qc.count} queries")
