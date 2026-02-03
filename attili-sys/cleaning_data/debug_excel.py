import pandas as pd

xlsx_path = "Fake News Alerts Database - 1.xlsx"

print("--- Reading with header=None (raw data first 5 rows) ---")
df = pd.read_excel(xlsx_path, header=None, nrows=5)
print(df)
