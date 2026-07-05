with open("rag/nlp/search.py", "r") as f:
    content = f.read()

content = content.replace('''<<<<<<< HEAD
        sorted_idx = np.argsort(sort_sim_np * -1, kind='stable')
=======
        sorted_idx = np.argsort(sim_np * -1, kind="stable")
>>>>>>> origin/infinitiflow-main''', '''        sorted_idx = np.argsort(sort_sim_np * -1, kind="stable")''')

with open("rag/nlp/search.py", "w") as f:
    f.write(content)
