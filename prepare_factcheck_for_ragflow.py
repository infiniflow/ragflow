from pathlib import Path


def extract_qa_from_article(content, filename):
    """Extract Q&A pairs from fact-check article"""
    lines = content.strip().split('\n')
    
    # Extract key fields
    category = ""
    title = ""
    main_content = []
    claim_section = []
    verification_section = []
    
    in_claim = False
    in_verification = False
    
    for line in lines:
        line = line.strip()
        
        if line.startswith("Category:"):
            category = line.replace("Category:", "").strip()
        elif line.startswith("Title:"):
            title = line.replace("Title:", "").strip()
        elif line in ["الادعاء:", "Claim:", "الإشاعة:"]:
            in_claim = True
            in_verification = False
        elif line in ["التحقق:", "Verification:", "الحقيقة:"]:
            in_verification = True
            in_claim = False
        elif in_claim and line and not line.startswith(("الرابط:", "المصدر:", "https://")):
            claim_section.append(line)
        elif in_verification and line and not line.startswith(("الرابط:", "المصدر:", "https://")):
            verification_section.append(line)
        elif not in_claim and not in_verification and line and not line.startswith(("Category:", "Title:", "الرابط:", "المصدر:", "المكان:", "الوسائط:", "التاريخ:", "الملكية الفكرية:")):
            # Main content (the story/explanation)
            if not line.startswith("http"):
                main_content.append(line)
    
    # Build Q&A pairs
    qa_pairs = []
    
    # Main Q&A: Title as question, content as answer
    if title and main_content:
        answer = " ".join(main_content)
        qa_pairs.append({
            "question": title,
            "answer": f"[{category}] {answer}",
            "metadata": f"Category: {category} | Source: {filename}"
        })
    
    # If there's a specific claim, create a Q&A for it
    if claim_section and verification_section:
        claim = " ".join(claim_section)
        verification = " ".join(verification_section)
        qa_pairs.append({
            "question": f"هل صحيح: {claim}",
            "answer": f"[{category}] {verification}",
            "metadata": f"Category: {category} | Source: {filename}"
        })
    
    return qa_pairs


def create_qa_batches(input_dir, output_dir, batch_size=20):
    """Create Q&A formatted batch files for RAGFlow"""
    
    input_path = Path(input_dir)
    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)
    
    # Get all markdown files
    files = sorted(input_path.glob("*.md"))
    print(f"Found {len(files)} files")
    
    all_qa_pairs = []
    
    # Process each file
    for file in files:
        try:
            with open(file, 'r', encoding='utf-8') as f:
                content = f.read()
            
            qa_pairs = extract_qa_from_article(content, file.name)
            all_qa_pairs.extend(qa_pairs)
            
        except Exception as e:
            print(f"Error processing {file.name}: {e}")
    
    print(f"Extracted {len(all_qa_pairs)} Q&A pairs")
    
    # Create batch files
    batch_num = 0
    for i in range(0, len(all_qa_pairs), batch_size):
        batch = all_qa_pairs[i:i+batch_size]
        output_file = output_path / f"factcheck_batch_{batch_num:03d}.txt"
        
        with open(output_file, 'w', encoding='utf-8') as out:
            for qa in batch:
                # RAGFlow Q&A format: Question on one line, Answer on next
                out.write(f"Q: {qa['question']}\n")
                out.write(f"A: {qa['answer']}\n")
                out.write(f"# {qa['metadata']}\n")
                out.write("\n" + "="*80 + "\n\n")
        
        batch_num += 1
        print(f"✓ Created {output_file.name} ({len(batch)} Q&A pairs)")
    
    print(f"\n✅ Done! Created {batch_num} batch files")
    print(f"📁 Output: {output_path}")
    
    return batch_num


# Run it
if __name__ == "__main__":
    create_qa_batches(
        input_dir="attili-sys/cleaning_data/ragflow_docs",
        output_dir="attili-sys/cleaning_data/ragflow_batches_qa",
        batch_size=20  # 20 articles per batch = ~10 batch files
    )
