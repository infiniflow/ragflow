<div align="center">
<a href="https://cloud.ragflow.io/">
<img src="web/src/assets/logo-with-text.svg" width="520" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md"><img alt="README in English" src="https://img.shields.io/badge/English-DFE0E5"></a>
  <a href="./README_zh.md"><img alt="简体中文版自述文件" src="https://img.shields.io/badge/简体中文-DFE0E5"></a>
  <a href="./README_tzh.md"><img alt="繁體版中文自述文件" src="https://img.shields.io/badge/繁體中文-DFE0E5"></a>
  <a href="./README_ja.md"><img alt="日本語のREADME" src="https://img.shields.io/badge/日本語-DFE0E5"></a>
  <a href="./README_ko.md"><img alt="한국어" src="https://img.shields.io/badge/한국어-DFE0E5"></a>
  <a href="./README_id.md"><img alt="Bahasa Indonesia" src="https://img.shields.io/badge/Bahasa Indonesia-DFE0E5"></a>
  <a href="./README_pt_br.md"><img alt="Português(Brasil)" src="https://img.shields.io/badge/Português(Brasil)-DFE0E5"></a>
  <a href="./README_fr.md"><img alt="README en Français" src="https://img.shields.io/badge/Français-DFE0E5"></a>
  <a href="./README_ar.md"><img alt="README in Arabic" src="https://img.shields.io/badge/Arabic-DBEDFA"></a>
  <a href="./README_tr.md"><img alt="Türkçe README" src="https://img.shields.io/badge/Türkçe-DFE0E5"></a>
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="follow on X(Twitter)">
    </a>
    <a href="https://cloud.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/Online-Demo-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/docker/pulls/infiniflow/ragflow?label=Docker%20Pulls&color=0db7ed&logo=docker&logoColor=white&style=flat-square" alt="docker pull infiniflow/ragflow:v0.25.1">
    </a>
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Latest%20Release" alt="Latest Release">
    </a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
        <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="license">
    </a>
    <a href="https://deepwiki.com/infiniflow/ragflow">
        <img alt="Ask DeepWiki" src="https://deepwiki.com/badge.svg">
    </a>
</p>

<h4 align="center">
  <a href="https://ragflow.io/docs/dev/">Document</a> |
  <a href="https://github.com/infiniflow/ragflow/issues/12241">Roadmap</a> |
  <a href="https://twitter.com/infiniflowai">Twitter</a> |
  <a href="https://discord.gg/NjYzJD3GM3">Discord</a> |
  <a href="https://cloud.ragflow.io">Demo</a>
</h4>

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/ragflow-octoverse.png" width="1200"/>
</div>

<div align="center">
<a href="https://trendshift.io/repositories/9064" target="_blank"><img src="https://trendshift.io/api/badge/repositories/9064" alt="infiniflow%2Fragflow | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>
</div>

<details open>
<summary><b>📕 جدول المحتويات</b></summary>

- 💡 [ما هو RAGFlow؟](#-what-is-ragflow)
- 🎮 [Demo](#-demo)
- 📌 [آخر التحديثات](#-latest-updates)
- 🌟 [الميزات الرئيسية](#-key-features)
- 🔎 [بنية النظام](#-system-architecture)
- 🎬 [ابدأ](#-get-started)
- 🔧 [التكوينات](#-configurations)
- 🔧 [إنشاء صورة Docker](#-build-a-docker-image)
- 🔨 [إطلاق الخدمة من المصدر للتطوير](#-launch-service-from-source-for-development)
- 📚 [التوثيق](#-documentation)
- 📜 [Roadmap](#-roadmap)
- 🏄 [المجتمع](#-community)
- 🙌 [مساهمة](#-contributing)

</details>

## 💡 ما هو RAGFlow؟

يُعد مشروع [RAGFlow](https://ragflow.io/) محركًا رائدًا ومفتوح المصدر للاسترجاع المعزز بالتوليد (<bdi dir="ltr">RAG</bdi>)، ويجمع أحدث تقنيات <bdi dir="ltr">RAG</bdi> مع قدرات الوكلاء لبناء طبقة سياق متقدمة لنماذج <bdi dir="ltr">LLMs</bdi>. يوفّر سير عمل <bdi dir="ltr">RAG</bdi> مبسّطًا وقابلًا للتكيّف مع المؤسسات بمختلف أحجامها. وبالاعتماد على [محرك سياق موحّد](https://ragflow.io/basics/what-is-agent-context-engine) وقوالب وكلاء جاهزة، يتيح <bdi dir="ltr">RAGFlow</bdi> للمطورين تحويل البيانات المعقّدة إلى أنظمة <bdi dir="ltr">AI</bdi> عالية الدقة وجاهزة للإنتاج بكفاءة وموثوقية.

## 🎮 Demo

جرّب النسخة التجريبية على [https://cloud.ragflow.io](https://cloud.ragflow.io).

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/chunking.gif" width="1200"/>
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/agentic-dark.gif" width="1200"/>
</div>

## 🔥 آخر التحديثات

- 24-04-2026 يدعم DeepSeek v4.
- 24-03-2026 [RAGFlow Skill on OpenClaw](https://clawhub.ai/yingfeng/ragflow-skill) — توفر مهارة رسمية للوصول إلى مجموعات بيانات RAGFlow عبر OpenClaw.
- 26-12-2025 يدعم ميزة "Memory" لوكلاء الذكاء الاصطناعي.
- 11-11-2025 يدعم Gemini 3 Pro.
- 12-11-2025 يدعم مزامنة البيانات من Confluence، S3، Notion، Discord، Google Drive.
- 23-10-2025 يدعم MinerU وDocling كطرق لتحليل المستندات.
- 15-10-2025 يدعم العرض الأوركسترالي pipeline.
- 08-08-2025 يدعم أحدث موديلات سلسلة OpenAI.
- 01-08-2025 يدعم سير العمل الوكيل وMCP.
- 23-05-2025 تمت إضافة مكون منفذ كود Python/JavaScript إلى Agent.
- 05-05-2025 يدعم الاستعلام بين اللغات.
- 19-03-2025 يدعم استخدام نموذج متعدد الوسائط لفهم الصور داخل ملفات PDF أو DOCX.

## 🎉 تابعونا

⭐️ قم بتمييز مستودعنا بنجمة لتبقى على اطلاع بالميزات والتحسينات الجديدة والمثيرة! احصل على إشعارات فورية بالجديد
الإصدارات! 🌟

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 الميزات الرئيسية

### 🍭 **"الجودة في الداخل، الجودة في الخارج"**

- [الفهم العميق للمستندات](./deepdoc/README.md) لاستخراج المعرفة من البيانات غير المنظمة
  ذات التنسيقات المعقدة.
- يجد "إبرة في كومة قش بيانات" من الرموز غير المحدودة حرفيًا.

### 🍱 **التقطيع القائم على القالب**

- ذكي وقابل للتفسير.
- الكثير من خيارات القالب للاختيار من بينها.

### 🌱 **استشهادات مؤرضة لتقليل الهلوسة**

- تصور تقطيع النص للسماح بالتدخل البشري.
- عرض سريع للمراجع الرئيسية والاستشهادات التي يمكن تتبعها لدعم الإجابات المبنية على أسس سليمة.

### 🍔 **التوافق مع مصادر البيانات غير المتجانسة**

- يدعم Word، والشرائح، وExcel، وtxt، والصور، والنسخ الممسوحة ضوئيًا، والبيانات المنظمة، وصفحات الويب، والمزيد.

### 🛀 **سير عمل RAG آلي وسهل**

- تنسيق RAG مبسط يلبي احتياجات الشركات الشخصية والكبيرة على حد سواء.
- نماذج LLMs قابلة للتكوين بالإضافة إلى نماذج embedding.
- الاستدعاء المتعدد المقترن بإعادة التصنيف المدمجة.
- APIs بديهي للتكامل السلس مع الأعمال.

## 🔎 هندسة النظام

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/user-attachments/assets/31b0dd6f-ca4f-445a-9457-70cb44a381b2" width="1000"/>
</div>

## 🎬 ابدأ

### 📝 المتطلبات الأساسية

- CPU >= 4 مراكز
- الرام >= 16 جيجا
- القرص >= 50 جيجا بايت
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- [gVisor](https://gvisor.dev/docs/user_guide/install/): مطلوب فقط إذا كنت تنوي استخدام ميزة منفذ التعليمات البرمجية (وضع الحماية) لـ RAGFlow.

> [!TIP]
> إذا لم تقم بتثبيت Docker على جهازك المحلي (Windows أو Mac أو Linux)، راجع [تثبيت Docker Engine](https://docs.docker.com/engine/install/).

### 🚀 بدء تشغيل الخادم

1. تأكد من `vm.max_map_count` >= 262144:

   > للتحقق من قيمة `vm.max_map_count`:
   >
   > ```bash
   > $ sysctl vm.max_map_count
   > ```
   >
   > أعد تعيين `vm.max_map_count` إلى قيمة 262144 على الأقل إذا لم تكن كذلك.
   >
   > ```bash
   > # In this case, we set it to 262144:
   > $ sudo sysctl -w vm.max_map_count=262144
   > ```
   >
   > سيتم إعادة ضبط هذا التغيير بعد إعادة تشغيل النظام. لضمان بقاء التغيير دائمًا، قم بإضافة أو تحديث
   > `vm.max_map_count` القيمة في **/etc/sysctl.conf** وفقًا لذلك:
   >
   > ```bash
   > vm.max_map_count=262144
   > ```
   >
2. استنساخ الريبو:

   ```bash
   $ git clone https://github.com/infiniflow/ragflow.git
   ```
3. ابدأ تشغيل الخادم باستخدام صور Docker المعدة مسبقًا:

> [!CAUTION]
> جميع الصور Docker مصممة لمنصات x86. لا نعرض حاليًا صور Docker لـ ARM64.
> إذا كنت تستخدم نظامًا أساسيًا ARM64، فاتبع [هذا الدليل](https://ragflow.io/docs/dev/build_docker_image) لإنشاء صورة Docker متوافقة مع نظامك.

> يقوم الأمر أدناه بتنزيل إصدار `v0.25.1` من الصورة RAGFlow Docker. راجع الجدول التالي للحصول على أوصاف لإصدارات RAGFlow المختلفة. لتنزيل إصدار RAGFlow مختلف عن `v0.25.1`، قم بتحديث المتغير `RAGFLOW_IMAGE` وفقًا لذلك في **docker/.env** قبل استخدام `docker compose` لبدء تشغيل الخادم.

```bash
   $ cd ragflow/docker

   # git checkout v0.25.1
   # Optional: use a stable tag (see releases: https://github.com/infiniflow/ragflow/releases)
   # This step ensures the **entrypoint.sh** file in the code matches the Docker image version.

   # Use CPU for DeepDoc tasks:
   $ docker compose -f docker-compose.yml up -d

   # To use GPU to accelerate DeepDoc tasks:
   # sed -i '1i DEVICE=gpu' .env
   # docker compose -f docker-compose.yml up -d
```

> ملاحظة: قبل `v0.22.0`، قدمنا ​​كلتا الصورتين بنماذج embedding وصورًا رفيعة بدون نماذج embedding. التفاصيل على النحو التالي:

| RAGFlow علامة الصورة | حجم الصورة (جيجابايت) | هل لديه نماذج embedding؟ | مستقر؟        |
|-------------------|-----------------|-----------------------|----------------|
| v0.21.1 | &approx;9 | ✔️ | إصدار مستقر |
| v0.21.1-slim | &approx;2 | ❌ | إصدار مستقر |

> بدءًا من `v0.22.0`، نقوم بشحن الإصدار النحيف فقط ولم نعد نلحق اللاحقة **-slim** بعلامة الصورة.

4. التحقق من حالة الخادم بعد تشغيل الخادم:

   ```bash
   $ docker logs -f docker-ragflow-cpu-1
   ```

   _النتيجة التالية تؤكد الإطلاق الناجح للنظام:_

   ```bash

         ____   ___    ______ ______ __
        / __ \ /   |  / ____// ____// /____  _      __
       / /_/ // /| | / / __ / /_   / // __ \| | /| / /
      / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /
     /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/

    * Running on all addresses (0.0.0.0)
   ```

   > إذا تخطيت خطوة التأكيد هذه وقمت بتسجيل الدخول مباشرة إلى RAGFlow، فقد يعرض متصفحك تنبيه `network abnormal`
   > خطأ لأنه في تلك اللحظة، قد لا تتم تهيئة RAGFlow بشكل كامل.
   >
5. في متصفح الويب الخاص بك، أدخل عنوان IP الخاص بالخادم الخاص بك وقم بتسجيل الدخول إلى RAGFlow.

   > باستخدام الإعدادات الافتراضية، ما عليك سوى إدخال `http://IP_OF_YOUR_MACHINE` (**من دون** رقم المنفذ) كإعداد افتراضي
   > HTTP يمكن حذف منفذ العرض `80` عند استخدام التكوينات الافتراضية.
   >
6. في [service_conf.yaml.template](./docker/service_conf.yaml.template)، حدد المصنع LLM المطلوب في `user_default_llm` وقم بالتحديث
   الحقل `API_KEY` مع مفتاح API المقابل.

   > راجع [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup) لمزيد من المعلومات.
   >

   _العرض بدأ!_

## 🔧 التكوينات

عندما يتعلق الأمر بتكوينات النظام، ستحتاج إلى إدارة الملفات التالية:

- [.env](./docker/.env): يحتفظ بالإعدادات الأساسية للنظام، مثل `SVR_HTTP_PORT`، `MYSQL_PASSWORD`، و
  `MINIO_PASSWORD`.
- [service_conf.yaml.template](./docker/service_conf.yaml.template): تكوين الخدمات الخلفية. سيتم ملء متغيرات البيئة في هذا الملف تلقائيًا عند بدء تشغيل الحاوية Docker. ستكون أي متغيرات بيئة تم تعيينها داخل حاوية Docker متاحة للاستخدام، مما يسمح لك بتخصيص سلوك الخدمة استنادًا إلى بيئة النشر.
- [docker-compose.yml](./docker/docker-compose.yml): يعتمد النظام على [docker-compose.yml](./docker/docker-compose.yml) لبدء التشغيل.

> يوفر الملف [./docker/README](./docker/README.md) وصفًا تفصيليًا لإعدادات البيئة والخدمة
> التكوينات التي يمكن استخدامها كـ `${ENV_VARS}` في ملف [service_conf.yaml.template](./docker/service_conf.yaml.template).

لتحديث منفذ العرض الافتراضي HTTP (80)، انتقل إلى [docker-compose.yml](./docker/docker-compose.yml) وقم بتغيير `80:80`
إلى `<YOUR_SERVING_PORT>:80`.

تتطلب تحديثات التكوينات المذكورة أعلاه إعادة تشغيل جميع الحاويات لتصبح سارية المفعول:

> ```bash
> $ docker compose -f docker-compose.yml up -d
> ```

### تبديل محرك المستندات من Elasticsearch إلى Infinity

RAGFlow يستخدم Elasticsearch بشكل افتراضي لتخزين النص الكامل والمتجهات. للتبديل إلى [Infinity](https://github.com/infiniflow/infinity/)، اتبع الخطوات التالية:

1. إيقاف كافة الحاويات قيد التشغيل:

   ```bash
   $ docker compose -f docker/docker-compose.yml down -v
   ```

> [!WARNING]
> `-v` سوف يحذف docker وحدات تخزين الحاوية، وسيتم مسح البيانات الموجودة.

2. اضبط `DOC_ENGINE` في **docker/.env** على `infinity`.
3. ابدأ الحاويات:

   ```bash
   $ docker compose -f docker-compose.yml up -d
   ```

> [!WARNING]
> التبديل إلى Infinity على جهاز Linux/arm64 غير مدعوم رسميًا بعد.

## 🔧 أنشئ صورة Docker

يبلغ حجم هذه الصورة حوالي 2 غيغابايت وتعتمد على خدمات LLM وembedding الخارجية.

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
docker build --platform linux/amd64 -f Dockerfile -t infiniflow/ragflow:nightly .
```

أو إذا كنت خلف وكيل، فيمكنك تمرير وسيطات الوكيل:

```bash
docker build --platform linux/amd64 \
  --build-arg http_proxy=http://YOUR_PROXY:PORT \
  --build-arg https_proxy=http://YOUR_PROXY:PORT \
  -f Dockerfile -t infiniflow/ragflow:nightly .
```

## 🔨 إطلاق الخدمة من المصدر للتطوير

1. قم بتثبيت `uv` و`pre-commit`، أو قم بتخطي هذه الخطوة إذا كانا مثبتين بالفعل:

   ```bash
   pipx install uv pre-commit
   ```
2. استنساخ الكود المصدري وتثبيت تبعيات بايثون:

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   uv sync --python 3.12 # install RAGFlow dependent python modules
   uv run python3 download_deps.py
   pre-commit install
   ```
3. قم بتشغيل الخدمات التابعة (MinIO وElasticsearch وRedis وMySQL) باستخدام Docker Compose:

   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

   أضف السطر التالي إلى `/etc/hosts` لحل كافة المضيفين المحددين في **docker/.env** إلى `127.0.0.1`:

   ```
   127.0.0.1       es01 infinity mysql minio redis sandbox-executor-manager
   ```
4. إذا لم تتمكن من الوصول إلى HuggingFace، فقم بتعيين متغير البيئة `HF_ENDPOINT` لاستخدام موقع مرآة:

   ```bash
   export HF_ENDPOINT=https://hf-mirror.com
   ```
5. إذا كان نظام التشغيل لديك لا يحتوي على jemalloc، فيرجى تثبيته على النحو التالي:

   ```bash
   # Ubuntu
   sudo apt-get install libjemalloc-dev
   # CentOS
   sudo yum install jemalloc
   # OpenSUSE
   sudo zypper install jemalloc
   # macOS
   sudo brew install jemalloc
   ```
6. إطلاق الخدمة الخلفية:

   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```
7. تثبيت تبعيات الواجهة الأمامية:

   ```bash
   cd web
   npm install
   ```
8. إطلاق خدمة الواجهة الأمامية:

   ```bash
   npm run dev
   ```

   _النتيجة التالية تؤكد الإطلاق الناجح للنظام:_

   ![](https://github.com/user-attachments/assets/0daf462c-a24d-4496-a66f-92533534e187)
9. أوقف خدمة الواجهة الأمامية والخلفية RAGFlow بعد اكتمال التطوير:

   ```bash
   pkill -f "ragflow_server.py|task_executor.py"
   ```

## 📚 التوثيق

- [البدء السريع](https://ragflow.io/docs/dev/)
- [التكوين](https://ragflow.io/docs/dev/configurations)
- [ملاحظات الإصدار](https://ragflow.io/docs/dev/release_notes)
- [أدلة المستخدم](https://ragflow.io/docs/category/user-guides)
- [أدلة المطورين](https://ragflow.io/docs/category/developer-guides)
- [المراجع](https://ragflow.io/docs/dev/category/references)
- [الأسئلة الشائعة](https://ragflow.io/docs/dev/faq)

## 📜 Roadmap

راجع [RAGFlow Roadmap 2026](https://github.com/infiniflow/ragflow/issues/12241)

## 🏄 المجتمع

- [Discord](https://discord.gg/NjYzJD3GM3)
- [Twitter](https://twitter.com/infiniflowai)
- [مناقشات جيثب](https://github.com/orgs/infiniflow/discussions)

## 🙌 المساهمة

RAGFlow يزدهر من خلال التعاون مفتوح المصدر. وبهذه الروح، فإننا نحتضن المساهمات المتنوعة من المجتمع.
إذا كنت ترغب في أن تكون جزءًا، فراجع [إرشادات المساهمة](https://ragflow.io/docs/dev/contributing) أولاً.
