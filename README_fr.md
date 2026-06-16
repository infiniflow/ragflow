<div align="center">
  <img src="web/public/logo-light.png" width="200" alt="MetaGrossAI logo">
  <h1>MetaGrossAI</h1>
</div>

## 💡 Qu'est-ce que MetaGrossAI ?
**MetaGrossAI** est un moteur de génération augmentée par la recherche (RAG) de premier plan qui fusionne une technologie RAG de pointe avec des capacités d'Agent pour créer une couche de contexte supérieure pour les LLM. Il offre un flux de travail RAG rationalisé et adaptable aux entreprises de toutes tailles. Propulsé par un moteur de contexte convergent et des modèles d'agents prédéfinis, MetaGrossAI permet aux développeurs de transformer des données complexes en systèmes d'IA haute fidélité, prêts pour la production, avec une efficacité et une précision exceptionnelles.

## 🌟 Fonctionnalités clés
### 🍭 **"La qualité en entrée détermine la qualité en sortie"**
- Extraction de connaissances basée sur la compréhension profonde de documents à partir de données non structurées.
- Trouve "l'aiguille dans une botte de foin de données" parmi un nombre illimité de tokens.

### 🍱 **Découpage (chunking) basé sur des modèles**
- Intelligent et explicable.
- Nombreuses options de modèles au choix.

### 🌱 **Citations fondées réduisant les hallucinations**
- Visualisation du découpage du texte pour permettre l'intervention humaine.
- Vue rapide des références clés et citations traçables pour étayer les réponses.

### 🍔 **Compatibilité avec des sources de données hétérogènes**
- Prend en charge Word, diapositives, Excel, txt, images, copies numérisées, données structurées, pages Web, etc.

### 🛀 **Flux de travail RAG automatisé et sans effort**
- LLM et modèles d'intégration configurables.
- API intuitives pour une intégration transparente avec l'entreprise.

## 🎬 Auto-hébergement
### 📝 Prérequis
- CPU >= 4 cores
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- Python >= 3.13

### 🚀 Démarrer le serveur
1. Assurez-vous que `vm.max_map_count` >= 262144 :
   ```bash
   $ sudo sysctl -w vm.max_map_count=262144
