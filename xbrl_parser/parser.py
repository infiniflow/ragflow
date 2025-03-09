import json
import os

import pandas as pd


# Function to read Company Facts data from a local file
def read_company_facts_from_file(file_path):
    if os.path.exists(file_path):
        with open(file_path, 'r') as file:
            return json.load(file)
    else:
        print(f"File {file_path} not found.")
        return None

# Function to parse JSON data into CSV
def parse_to_csv(data, output_file):
    records = []

    if "facts" in data:
        accn = data.get("accn")  # Extracting the Accession Number (accn)
        
        for taxonomy, facts in data["facts"].items():
            for concept, details in facts.items():
                for unit, items in details.get("units", {}).items():
                    for entry in items:
                        records.append({
                            "cik": data.get("cik"),
                            "company_name": data.get("entityName"),
                            "taxonomy": taxonomy,
                            "concept": concept,
                            "unit": unit,
                            "value": entry.get("val"),
                            "start_date": entry.get("start"),
                            "end_date": entry.get("end"),
                            "accn": entry.get("accn"),
                            "fy": entry.get("fy"),
                            "fp": entry.get("fp"),
                            "form": entry.get("form"),
                            "filed_date": entry.get("filed"),
                            "frame": entry.get("frame")
                        })

    df = pd.DataFrame(records)
    df.to_csv(output_file, index=False)
    print(f"Data saved to {output_file}")

# Example usage
if __name__ == "__main__":
    # Set the local file path for the company facts JSON
    file_path = "apple.json"  # Path to your local JSON file

    # Read data from the local file
    data = read_company_facts_from_file(file_path)

    if data:
        # Parse and save the data to CSV
        parse_to_csv(data, "company_facts_" + file_path[:-5] + ".csv")