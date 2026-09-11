[English](./README.md) | [简体中文](./README_zh.md) | Türkçe

# Eklentiler

Bu klasör, RAGFlow'un eklenti mekanizmasını içerir.

RAGFlow, `embedded_plugins` alt klasöründen eklentileri özyinelemeli olarak yükleyecektir.

## Desteklenen eklenti türleri

Şu anda desteklenen tek eklenti türü `llm_tools`'dur.

- `llm_tools`: LLM'nin çağırması için bir araç.

## Eklenti nasıl eklenir

Bir LLM araç eklentisi eklemek basittir: bir eklenti dosyası oluşturun, içine `LLMToolPlugin` sınıfından türetilmiş bir sınıf koyun, ardından `get_metadata` ve `invoke` metodlarını uygulayın.

- `get_metadata` metodu: Bu metod, aracın açıklamasını içeren bir `LLMToolMetadata` nesnesi döndürür.
Açıklama, LLM'ye çağrı için ve RAGFlow web ön yüzüne görüntüleme amacıyla sağlanacaktır.

- `invoke` metodu: Bu metod, LLM tarafından üretilen parametreleri kabul eder ve aracın yürütme sonucunu içeren bir `str` döndürür.
Bu aracın tüm yürütme mantığı bu metoda konulmalıdır.

RAGFlow'u başlattığınızda, günlükte eklentinizin yüklendiğini göreceksiniz:

```
2025-05-15 19:29:08,959 INFO     34670 Recursively importing plugins from path `/some-path/ragflow/agent/plugin/embedded_plugins`
2025-05-15 19:29:08,960 INFO     34670 Loaded llm_tools plugin BadCalculatorPlugin version 1.0.0
```

Veya eklentinizi düzeltmeniz gereken hatalar da içerebilir.

### Örnek

Yanlış cevaplar veren bir hesap makinesi aracı ekleyerek eklenti ekleme sürecini göstereceğiz.

Önce, `embedded_plugins/llm_tools` klasörü altında `bad_calculator.py` adında bir eklenti dosyası oluşturun.

Ardından, `LLMToolPlugin` temel sınıfından türetilmiş bir `BadCalculatorPlugin` sınıfı oluşturuyoruz:

```python
class BadCalculatorPlugin(LLMToolPlugin):
    _version_ = "1.0.0"
```

`_version_` alanı zorunludur ve eklentinin sürüm numarasını belirtir.

Hesap makinemizin girdileri olarak `a` ve `b` olmak üzere iki sayısı vardır, bu yüzden `BadCalculatorPlugin` sınıfımıza aşağıdaki `invoke` metodunu ekliyoruz:

```python
def invoke(self, a: int, b: int) -> str:
    return str(a + b + 100)
```

`invoke` metodu LLM tarafından çağrılacaktır. Birçok parametreye sahip olabilir, ancak dönüş tipi `str` olmalıdır.

Son olarak, LLM'ye `bad_calculator` aracımızı nasıl kullanacağını anlatmak için bir `get_metadata` metodu eklememiz gerekiyor:

```python
@classmethod
def get_metadata(cls) -> LLMToolMetadata:
    return {
        # Bu aracın adı, LLM'ye sağlanır
        "name": "bad_calculator",
        # Bu aracın görüntüleme adı, RAGFlow ön yüzüne sağlanır
        "displayName": "$t:bad_calculator.name",
        # Bu aracın kullanım açıklaması, LLM'ye sağlanır
        "description": "A tool to calculate the sum of two numbers (will give wrong answer)",
        # Bu aracın açıklaması, RAGFlow ön yüzüne sağlanır
        "displayDescription": "$t:bad_calculator.description",
        # Bu aracın parametreleri
        "parameters": {
            # Birinci parametre - a
            "a": {
                # Parametre tipi, seçenekler: number, string veya LLM'nin tanıyabileceği herhangi bir tip
                "type": "number",
                # Bu parametrenin açıklaması, LLM'ye sağlanır
                "description": "The first number",
                # Bu parametrenin açıklaması, RAGFlow ön yüzüne sağlanır
                "displayDescription": "$t:bad_calculator.params.a",
                # Bu parametrenin zorunlu olup olmadığı
                "required": True
            },
            # İkinci parametre - b
            "b": {
                "type": "number",
                "description": "The second number",
                "displayDescription": "$t:bad_calculator.params.b",
                "required": True
            }
        }
```

`get_metadata` metodu bir `classmethod`'dur. Bu aracın açıklamasını LLM'ye sağlayacaktır.

`display` ile başlayan alanlar özel bir gösterim kullanabilir: `$t:xxx`, bu gösterim RAGFlow ön yüzündeki uluslararasılaştırma (i18n) mekanizmasını kullanarak `llmTools` kategorisinden metin alır. Bu gösterimi kullanmazsanız, ön yüz buraya yazdığınız metni doğrudan gösterecektir.

Artık aracımız hazırdır. `Yanıt Üret` bileşeninde seçip deneyebilirsiniz.
