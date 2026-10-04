<div align="center">
<a href="https://cloud.ragflow.io/">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow/main/web/src/assets/logo-with-text.svg" width="520" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md"><img alt="README in English" src="https://img.shields.io/badge/English-DFE0E5"></a>
  <a href="./README_zh.md"><img alt="简体中文版自述文件" src="https://img.shields.io/badge/简体中文-DFE0E5"></a>
  <a href="./README_tzh.md"><img alt="繁體版中文自述文件" src="https://img.shields.io/badge/繁體中文-DFE0E5"></a>
  <a href="./README_ja.md"><img alt="日本語のREADME" src="https://img.shields.io/badge/日本語-DFE0E5"></a>
  <a href="./README_ko.md"><img alt="한국어" src="https://img.shields.io/badge/한국어-DFE0E5"></a>
  <a href="./README_fr.md"><img alt="README en Français" src="https://img.shields.io/badge/Français-DFE0E5"></a>
  <a href="./README_id.md"><img alt="Bahasa Indonesia" src="https://img.shields.io/badge/Bahasa Indonesia-DFE0E5"></a>
  <a href="./README_pt_br.md"><img alt="Português(Brasil)" src="https://img.shields.io/badge/Português(Brasil)-DFE0E5"></a>
  <a href="./README_ar.md"><img alt="README in Arabic" src="https://img.shields.io/badge/Arabic-DBEDFA"></a>
  <a href="./README_tr.md"><img alt="Türkçe README" src="https://img.shields.io/badge/Türkçe-DFE0E5"></a>
  <a href="./README_ru.md"><img alt="Русская версия README" src="https://img.shields.io/badge/Русский-DFE0E5"></a>
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="follow on X(Twitter)">
    </a>
    <a href="https://cloud.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/Get-Started-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/infiniflow/ragflow-stats/main/badges/docker-pulls.json&style=flat-square&logo=docker&logoColor=white" alt="RAGFlow Docker image downloads">
    </a>
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Latest%20Release" alt="Latest Release">
    </a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
        <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="license">
    </a>
</p>

<h4 align="center">
  <a href="https://cloud.ragflow.io">Cloud</a> |
  <a href="https://ragflow.io/docs/dev/">Document</a> |
  <a href="https://github.com/infiniflow/ragflow/issues/12241">Roadmap</a> |
  <a href="https://discord.gg/NjYzJD3GM3">Discord</a>
</h4>

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="RAGFlow in the GitHub Octoverse" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/ragflow-octoverse.png" width="1200"/>
</div>

<div align="center">
<a href="https://trendshift.io/repositories/9064" target="_blank"><img src="https://trendshift.io/api/badge/repositories/9064" alt="infiniflow%2Fragflow | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>
</div>

<details open>
<summary><b>📕 جدول المحتويات</b></summary>

- 💡 [ما هو RAGFlow؟](#-ما-هو-ragflow)
- 🎮 [ابدأ](#-ابدأ)
- 📌 [آخر التحديثات](#-آخر-التحديثات)
- 🌟 [الميزات الرئيسية](#-الميزات-الرئيسية)
- 🔎 [بنية النظام](#-هندسة-النظام)
- 🎬 [الاستضافة الذاتية](#-الاستضافة-الذاتية)
- 🔧 [التكوينات](#-التكوينات)
- 🔧 [إنشاء صورة Docker](#-أنشئ-صورة-docker)
- 🔨 [إطلاق الخدمة من المصدر للتطوير](#-إطلاق-الخدمة-من-المصدر-للتطوير)
- 📚 [التوثيق](#-التوثيق)
- 📜 [Roadmap](#-roadmap)
- 🏄 [المجتمع](#-المجتمع)
- 🙌 [مساهمة](#-المساهمة)

</details>

## 💡 ما هو RAGFlow؟

يُعد مشروع [RAGFlow](https://ragflow.io/) محركًا رائدًا ومفتوح المصدر للاسترجاع المعزز بالتوليد (<bdi dir="ltr">RAG</bdi>)، ويجمع أحدث تقنيات <bdi dir="ltr">RAG</bdi> مع قدرات الوكلاء لبناء طبقة سياق متقدمة لنماذج <bdi dir="ltr">LLMs</bdi>. يوفّر سير عمل <bdi dir="ltr">RAG</bdi> مبسّطًا وقابلًا للتكيّف مع المؤسسات بمختلف أحجامها. وبالاعتماد على [محرك سياق موحّد](https://ragflow.io/basics/what-is-agent-context-engine) وقوالب وكلاء جاهزة، يتيح <bdi dir="ltr">RAGFlow</bdi> للمطورين تحويل البيانات المعقّدة إلى أنظمة <bdi dir="ltr">AI</bdi> عالية الدقة وجاهزة للإنتاج بكفاءة وموثوقية.

## 🎮 ابدأ

جرّب النسخة التجريبية على [https://cloud.ragflow.io](https://cloud.ragflow.io).

للنشر محليًا، راجع قسم [النشر المحلي](#-الاستضافة-الذاتية).

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="Chunking demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/chunking.gif" width="1200"/>
<img alt="Agentic workflow demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/agentic-dark.gif" width="1200"/>
</div>

## 🔥 آخر التحديثات

- 2026-09-29 إصدار RAGFlow 1.0.0-rc1.

- 2026-09-10 إضافة استيعاب محتوى الويب عبر خرائط المواقع.
- 2026-08-19 إطلاق Knowledge Compilation لإنشاء Wiki وGraph وTree وPageIndex وMind Map وTimeline وSkills على مستوى المستند ومجموعة البيانات.
- 2026-08-19 إطلاق Agentic RAG مع أوضاع التفكير Low وMedium وHigh وUltra.
- 2026-07-02 إضافة استيعاب مصادر بيانات Google BigQuery والمزامنة التدريجية.
- 2026-06-29 إضافة قنوات دردشة WhatsApp وDingTalk وWeCom.
- 2026-05-26 إضافة مكوّن Browser لتمكين Agents من تصفح صفحات الويب والتفاعل معها.
- 2026-04-21 إضافة سبعة قوالب مضمّنة لخطوط استيعاب البيانات.
- 2026-04-21 إضافة نشر تطبيقات Agent وتنفيذ التعليمات البرمجية في Sandbox وإنشاء المخططات.
- 2026-04-21 إضافة تخزين الذاكرة واسترجاعها على مستوى المستخدم.

راجع [ملاحظات الإصدار الكاملة](./docs/release_notes.md) لمزيد من التحديثات.

## 🎉 تابعونا

⭐️ قم بتمييز مستودعنا بنجمة لتبقى على اطلاع بالميزات والتحسينات الجديدة والمثيرة! احصل على إشعارات فورية بالجديد
الإصدارات! 🌟

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="RAGFlow feature updates" src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 الميزات الرئيسية

### 🍭 **"الجودة في الداخل، الجودة في الخارج"**

- الفهم العميق للمستندات لاستخراج المعرفة من البيانات غير المنظمة
  ذات التنسيقات المعقدة.
- يجد "إبرة في كومة قش بيانات" من الرموز غير المحدودة حرفيًا.

### 🍱 **التقطيع القائم على القالب**

- ذكي وقابل للتفسير.
- الكثير من خيارات القالب للاختيار من بينها.

### 🧩 **تجميع المعرفة (Knowledge Compilation)**

- يحوّل المحتوى على مستوى المستند ومجموعة البيانات إلى مخرجات معرفية منظمة، مثل Wiki وGraph وTree وPageIndex وMind Map وTimeline وSkills.
- يتيح ضبط نماذج التجميع وقواعد المعالجة، ثم عرض المخرجات وتحديثها وإعادة إنشائها.

### 🧠 **الاسترجاع الوكيلي (Agentic Retrieval)**

- يحلل الأسئلة المعقدة، ويقسمها عند الحاجة، ويسترجع المعرفة ويتحقق من الأدلة عبر خطوات متعددة.
- تدعم أوضاع التفكير Low وMedium وHigh وUltra ضبط عمق الاسترجاع والاستدلال حسب تعقيد السؤال.

### ⚙️ **بنية خدمات Go الأصلية**

- توفر خدمة Go موحدة API وAdmin وIngestor وSyncer. يعمل DeepDoc داخل عملية Go لتحليل التخطيط وOCR والتعرف على الجداول.
- تستدعي خدمات Go مكتبات تحليل المستندات الأصلية وONNX Runtime عبر CGO. ويمكن تفعيل MCP وSandbox Executor عند الحاجة.

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
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/structure.jpg" alt="بنية RAGFlow" width="1000" />
</div>

## 🎬 الاستضافة الذاتية

### 🐳 نشر Docker

#### 📝 متطلبات نشر Docker

- التكوين الابتدائي الموصى به: 4 أنوية CPU وذاكرة RAM بسعة 16 GB ومساحة قرص متاحة تبلغ 50 GB. تختلف المتطلبات الفعلية حسب محرك المستندات وحجم البيانات ومهام التحليل والتزامن؛ وقد تتطلب النماذج المحلية والمكونات الاختيارية الأخرى موارد إضافية.
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- [gVisor](https://gvisor.dev/docs/user_guide/install/): مطلوب فقط عند استخدام Sandbox للحاويات Self-Managed.

لا يتطلب نشر Docker تثبيت Go على المضيف. يتطلب Self-Managed container Sandbox تثبيت gVisor وإعداده؛ أما موفرو Sandbox الآخرون فلا يتطلبون تثبيت gVisor على مضيف RAGFlow.

> [!TIP]
> إذا لم تقم بتثبيت Docker على جهازك المحلي (Windows أو Mac أو Linux)، راجع [تثبيت Docker Engine](https://docs.docker.com/engine/install/).

#### 🚀 بدء تشغيل الخادم

1. تأكد من `vm.max_map_count` >= 262144:

   > للتحقق من قيمة `vm.max_map_count`:
   >
   > ```bash
   > sysctl vm.max_map_count
   > ```
   >
   > أعد تعيين `vm.max_map_count` إلى قيمة 262144 على الأقل إذا لم تكن كذلك.
   >
   > ```bash
   > # In this case, we set it to 262144:
   > sudo sysctl -w vm.max_map_count=262144
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
   git clone https://github.com/infiniflow/ragflow.git
   ```
3. انتقل إلى وسم إصدار Go وشغّل صورة Go الجاهزة باستخدام Docker Compose:

> [!CAUTION]
> جميع الصور Docker مصممة لمنصات x86. لا نعرض حاليًا صور Docker لـ ARM64.
> إذا كنت تستخدم نظامًا أساسيًا ARM64، فاتبع [هذا الدليل](https://ragflow.io/docs/dev/build_docker_image) لإنشاء صورة Docker متوافقة مع نظامك.



   الدخول إلى دليل نشر Docker.

   ```bash
   cd ragflow/docker
   ```

   التبديل إلى وسم إصدار Go v1.0.0-rc1.

   ```bash
   git checkout v1.0.0-rc1
   ```

   تشغيل خدمات Go وتبعياتها في الخلفية.

   ```bash
   docker compose -f docker-compose.yml up -d
   ```

   في إعداد MySQL الافتراضي، تنفّذ نقطة دخول صورة Go ترحيلات قاعدة البيانات أولًا، ثم تشغّل Syncer وAdmin وAPI وIngestor عبر `bin/ragflow_server`.

> يستخدم DeepDoc في الإصدار مفتوح المصدر 1.0 استدلال CPU لتحليل التخطيط وOCR والتعرف على الجداول.

4. تحقق من حالة الخدمات وجاهزية API بعد بدء التشغيل:

   ```bash
   docker ps
   ```

   يعرض الأمر أعلاه حالة الخدمات التابعة. لا يعرّف RAGFlow فحص صحة Compose؛ تحقق من الجاهزية عبر API:

   ```bash
   curl -f http://localhost/api/v1/system/healthz
   ```

   تشير استجابة HTTP 200 إلى الجاهزية. إذا غيّرت `SVR_WEB_HTTP_PORT`، فاستخدم ذلك المنفذ في عنوان URL لفحص الصحة. إذا فشل بدء التشغيل، فافحص سجلات الخدمة المعنية باستخدام `docker logs --tail 50 <service>`.

5. في متصفح الويب الخاص بك، أدخل عنوان IP الخاص بالخادم الخاص بك وقم بتسجيل الدخول إلى RAGFlow.

   > باستخدام الإعدادات الافتراضية، ما عليك سوى إدخال `http://IP_OF_YOUR_MACHINE` (**من دون** رقم المنفذ) كإعداد افتراضي
   > HTTP يمكن حذف منفذ العرض `80` عند استخدام التكوينات الافتراضية.
   >
6. بعد تسجيل الدخول، أضف LLM ونموذج embedding ونموذج reranker من صفحة موفري النماذج، ثم أدخل اسم النموذج وعنوان الخدمة ومفتاح API.

   > راجع [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup) لمزيد من المعلومات.
   >

   _العرض بدأ!_

#### ⚙️ إعداد Docker وتعديله

يستخدم نشر Go عبر Docker الملفين `docker/.env` و`docker/docker-compose.yml`، ويستخدم Kvrocks لتخزين ذاكرة التخزين المؤقت ونقاط التحقق، كما يستخدم NATS JetStream كقائمة انتظار للرسائل. لتعديل الصورة والمنافذ وكلمات المرور ومحرك المستندات ومصدر صور النماذج، اتبع [دليل إعداد Docker](./docker/README.md). ولقيود المنصات ومتطلبات macOS، راجع [دليل بناء صورة Go ودعم المنصات](./docs/develop/build_docker_image.mdx).

عند تبديل محرك المستندات أو تعديل الإعدادات وإعادة تشغيل الخدمات أو الاحتفاظ بالبيانات الحالية أو حذفها، اتبع أيضًا دليل إعداد Docker أعلاه.

### 🔨 إطلاق الخدمة من المصدر للتطوير

#### 📝 متطلبات البناء من المصدر

1. ثبّت إصدار Go المحدد في `go.mod` (حاليًا Go 1.27)، وClang 20 وLLD 20 وCMake 4.0 أو أحدث وملفات تطوير PCRE2. تعتمد خدمات Go على CGO والمكتبات الأصلية؛ ويضبط [build.sh](./build.sh) خيارات البناء اللازمة.
2. استنسخ المستودع وجهّز المكتبات الأصلية وملفات النماذج المطلوبة للبناء، ثم ابنِ خدمات Go:

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   ```

   ```bash
   python3 -m venv /tmp/ragflow-go-download-venv
   /tmp/ragflow-go-download-venv/bin/python -m pip install requests huggingface-hub
   /tmp/ragflow-go-download-venv/bin/python ragflow_deps/download_deps.py
   bash build.sh --all
   ```

   يجهّز البرنامج النصي المكتبات الأصلية وموارد النماذج اللازمة لبناء Go، ويحتاج إلى `requests` و`huggingface-hub`. يمكن تخطي هذه الخطوة إذا جُهزت الموارد نفسها بطريقة أخرى. عند التشغيل من جذر المستودع، تعثر خدمات Go تلقائيًا على `internal/rag/res/deepdoc`؛ وللتشغيل من دليل آخر، اضبط `DEEPDOC_MODEL_DIR` على المسار المطلق لذلك الدليل.
3. ابدأ الخدمات التابعة المطلوبة (Elasticsearch وMySQL وMinIO وNATS وKvrocks وClickHouse) باستخدام Docker Compose:

   ```bash
   sudo sysctl -w vm.max_map_count=262144
   docker compose --env-file docker/.env -f docker/docker-compose-base.yml \
     up -d --wait es01 mysql minio nats kvrocks clickhouse
   ```

   تتصل خدمات Go التي تعمل من المصدر بـ Kvrocks عبر `localhost:6379`، ولا يتطلب التكوين المرفق تعديل `/etc/hosts`.

4. بعد اكتمال ترحيل قاعدة البيانات، شغّل الخدمات بالترتيب. نفّذ كل أمر في نافذة طرفية مستقلة ومن جذر المستودع، واترك نوافذ الخدمات الأربع مفتوحة:

   ```bash
   ./bin/ragflow_server --migrate
   ```

   ```bash
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --admin
   ```

   ```bash
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --ingestor
   ```

   ```bash
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --syncer
   ```

   ```bash
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --api
   ```

   تعمل أوضاع التشغيل كما يلي:

   - `--migrate`: ينفذ ترحيلات قاعدة البيانات ثم يخرج.
   - `--admin`: يشغّل خدمة Admin للإدارة والتهيئة.
   - `--ingestor`: يشغّل خدمة Ingestor لمهام استيعاب البيانات وتحليلها.
   - `--syncer`: يشغّل خدمة Syncer لمهام مزامنة البيانات.
   - `--api`: يشغّل خدمة API لواجهة الويب وSDK والعملاء الخارجيين.

   يُستخدم `RAGFLOW_DEV_MODE=true` للتطوير فقط؛ فهو يعطّل فحص الرجوع بين إصدار الكود وإصدار ترحيل قاعدة البيانات، ولا ينفذ الترحيلات أو يغير المخطط. لا تستخدمه في الإنتاج. شغّل Admin قبل الخدمات الأخرى. بعد الترحيل، يشغّل `RAGFLOW_DEV_MODE=true bash build.sh --run` خدمات Admin وIngestor وAPI، لكنه لا يشغّل Syncer؛ شغّل Syncer منفصلًا باستخدام `RAGFLOW_DEV_MODE=true ./bin/ragflow_server --syncer` لتشغيل سلسلة الخدمات كاملة.
5. ثبّت Node.js وnpm وشغّل واجهة React فقط عند تطوير الواجهة الأمامية:

   ```bash
   cd web
   npm install
   API_PROXY_SCHEME=go npm run dev
   ```

   تحقق من جاهزية Go API من نافذة طرفية أخرى:

   ```bash
   curl -f http://127.0.0.1:9380/api/v1/system/healthz
   ```

   تعني استجابة HTTP 200 أن API يستجيب. عند انتهاء التطوير، اضغط `Ctrl+C` في كل نافذة خدمة. لإيقاف الخدمات التابعة مع الاحتفاظ بالحاويات، شغّل `docker compose --env-file docker/.env -f docker/docker-compose-base.yml stop es01 mysql minio nats kvrocks clickhouse`. ولإزالة حاويات الخدمات التابعة وشبكة Compose مع الاحتفاظ بوحدات التخزين المسماة، شغّل `docker compose --env-file docker/.env -f docker/docker-compose-base.yml down`.

راجع [تشغيل الخدمة من المصدر](./docs/develop/launch_ragflow_from_source.md) لمزيد من التفاصيل.

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
- [X](https://x.com/infiniflowai)
- [مناقشات جيثب](https://github.com/orgs/infiniflow/discussions)

## 🙌 المساهمة

RAGFlow يزدهر من خلال التعاون مفتوح المصدر. وبهذه الروح، فإننا نحتضن المساهمات المتنوعة من المجتمع.
إذا كنت ترغب في أن تكون جزءًا، فراجع [إرشادات المساهمة](https://ragflow.io/docs/dev/contributing) أولاً.
