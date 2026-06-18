<div align="center">
  <img src="web/public/logo-light.png" width="200" alt="MetaGrossAI logo">
  <h1>MetaGrossAI</h1>
</div>

## 💡 O que é MetaGrossAI?
**MetaGrossAI** é um mecanismo líder em Geração Aumentada por Recuperação (RAG) que funde a tecnologia RAG de ponta com recursos de Agente para criar uma camada de contexto superior para LLMs. Ele oferece um fluxo de trabalho RAG simplificado, adaptável a empresas de qualquer escala. Alimentado por um mecanismo de contexto convergente e modelos de agentes pré-construídos, o MetaGrossAI permite que os desenvolvedores transformem dados complexos em sistemas de IA de alta fidelidade e prontos para produção, com eficiência e precisão excepcionais.

## 🌟 Principais Recursos
### 🍭 **"Qualidade na entrada, qualidade na saída"**
- Extração de conhecimento baseada na compreensão profunda de documentos a partir de dados não estruturados com formatos complexos.
- Encontra "uma agulha em um palheiro de dados" de tokens literalmente ilimitados.

### 🍱 **Fragmentação (chunking) baseada em modelos**
- Inteligente e explicável.
- Várias opções de modelos para escolher.

### 🌱 **Citações fundamentadas com redução de alucinações**
- Visualização da fragmentação do texto para permitir a intervenção humana.
- Visualização rápida das principais referências e citações rastreáveis para apoiar respostas fundamentadas.

### 🍔 **Compatibilidade com fontes de dados heterogêneas**
- Suporta Word, slides, excel, txt, imagens, cópias digitalizadas, dados estruturados, páginas da web e muito mais.

## 🎬 Auto-hospedagem
### 📝 Pré-requisitos
- CPU >= 4 cores
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- Python >= 3.13

### 🚀 Iniciando o servidor
1. Certifique-se de que `vm.max_map_count` >= 262144:
   ```bash
   $ sudo sysctl -w vm.max_map_count=262144
