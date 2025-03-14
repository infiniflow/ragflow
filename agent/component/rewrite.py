#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
from abc import ABC
from agent.component import GenerateParam, Generate
from rag.prompts import full_question


class RewriteQuestionParam(GenerateParam):
    """
    Define the QuestionRewrite component parameters.
    """

    def __init__(self):
        super().__init__()
        self.temperature = 0.9
        self.prompt = ""
        self.language = ""

    def check(self):
        super().check()


class RewriteQuestion(Generate, ABC):
    component_name = "RewriteQuestion"

    def _run(self, history, **kwargs):
        hist = self._canvas.get_history(self._param.message_history_window_size)
        query = self.get_input()
        query = str(query["content"][0]) if "content" in query else ""
        messages = [h for h in hist if h["role"]!="system"]
        if messages[-1]["role"] != "user":
            messages.append({"role": "user", "content": query})
        ans = full_question(self._canvas.get_tenant_id(), self._param.llm_id, messages, self.gen_lang(self._param.language))
        self._canvas.history.pop()
        self._canvas.history.append(("user", ans))
        return RewriteQuestion.be_output(ans)

    @staticmethod
    def gen_lang(language):
        # convert code lang to language word for the prompt
        language_dict = {'af': 'Afrikaans', 'ak': 'Akan', 'sq': 'Albanian', 'ws': 'Samoan', 'am': 'Amharic',
                         'ar': 'Arabic', 'hy': 'Armenian', 'az': 'Azerbaijani', 'eu': 'Basque', 'be': 'Belarusian',
                         'bem': 'Bemba', 'bn': 'Bengali', 'bh': 'Bihari',
                         'xx-bork': 'Bork', 'bs': 'Bosnian', 'br': 'Breton', 'bg': 'Bulgarian', 'bt': 'Bhutani',
                         'km': 'Cambodian', 'ca': 'Catalan', 'chr': 'Cherokee', 'ny': 'Chichewa', 'zh-cn': 'Chinese',
                         'zh-tw': 'Chinese', 'co': 'Corsican',
                         'hr': 'Croatian', 'cs': 'Czech', 'da': 'Danish', 'nl': 'Dutch', 'xx-elmer': 'Elmer',
                         'en': 'English', 'eo': 'Esperanto', 'et': 'Estonian', 'ee': 'Ewe', 'fo': 'Faroese',
                         'tl': 'Filipino', 'fi': 'Finnish', 'fr': 'French',
                         'fy': 'Frisian', 'gaa': 'Ga', 'gl': 'Galician', 'ka': 'Georgian', 'de': 'German',
                         'el': 'Greek', 'kl': 'Greenlandic', 'gn': 'Guarani', 'gu': 'Gujarati', 'xx-hacker': 'Hacker',
                         'ht': 'Haitian Creole', 'ha': 'Hausa', 'haw': 'Hawaiian',
                         'iw': 'Hebrew', 'hi': 'Hindi', 'hu': 'Hungarian', 'is': 'Icelandic', 'ig': 'Igbo',
                         'id': 'Indonesian', 'ia': 'Interlingua', 'ga': 'Irish', 'it': 'Italian', 'ja': 'Japanese',
                         'jw': 'Javanese', 'kn': 'Kannada', 'kk': 'Kazakh', 'rw': 'Kinyarwanda',
                         'rn': 'Kirundi', 'xx-klingon': 'Klingon', 'kg': 'Kongo', 'ko': 'Korean', 'kri': 'Krio',
                         'ku': 'Kurdish', 'ckb': 'Kurdish (Sorani)', 'ky': 'Kyrgyz', 'lo': 'Laothian', 'la': 'Latin',
                         'lv': 'Latvian', 'ln': 'Lingala', 'lt': 'Lithuanian',
                         'loz': 'Lozi', 'lg': 'Luganda', 'ach': 'Luo', 'mk': 'Macedonian', 'mg': 'Malagasy',
                         'ms': 'Malay', 'ml': 'Malayalam', 'mt': 'Maltese', 'mv': 'Maldivian', 'mi': 'Maori',
                         'mr': 'Marathi', 'mfe': 'Mauritian Creole', 'mo': 'Moldavian', 'mn': 'Mongolian',
                         'sr-me': 'Montenegrin', 'my': 'Burmese', 'ne': 'Nepali', 'pcm': 'Nigerian Pidgin',
                         'nso': 'Northern Sotho', 'no': 'Norwegian', 'nn': 'Norwegian Nynorsk', 'oc': 'Occitan',
                         'or': 'Oriya', 'om': 'Oromo', 'ps': 'Pashto', 'fa': 'Persian',
                         'xx-pirate': 'Pirate', 'pl': 'Polish', 'pt': 'Portuguese', 'pt-br': 'Portuguese (Brazilian)',
                         'pt-pt': 'Portuguese (Portugal)', 'pa': 'Punjabi', 'qu': 'Quechua', 'ro': 'Romanian',
                         'rm': 'Romansh', 'nyn': 'Runyankole', 'ru': 'Russian', 'gd': 'Scots Gaelic',
                         'sr': 'Serbian', 'sh': 'Serbo-Croatian', 'st': 'Sesotho', 'tn': 'Setswana',
                         'crs': 'Seychellois Creole', 'sn': 'Shona', 'sd': 'Sindhi', 'si': 'Sinhalese', 'sk': 'Slovak',
                         'sl': 'Slovenian', 'so': 'Somali', 'es': 'Spanish', 'es-419': 'Spanish (Latin America)',
                         'su': 'Sundanese',
                         'sw': 'Swahili', 'sv': 'Swedish', 'tg': 'Tajik', 'ta': 'Tamil', 'tt': 'Tatar', 'te': 'Telugu',
                         'th': 'Thai', 'ti': 'Tigrinya', 'to': 'Tongan', 'lua': 'Tshiluba', 'tum': 'Tumbuka',
                         'tr': 'Turkish', 'tk': 'Turkmen', 'tw': 'Twi',
                         'ug': 'Uyghur', 'uk': 'Ukrainian', 'ur': 'Urdu', 'uz': 'Uzbek', 'vu': 'Vanuatu',
                         'vi': 'Vietnamese', 'cy': 'Welsh', 'wo': 'Wolof', 'xh': 'Xhosa', 'yi': 'Yiddish',
                         'yo': 'Yoruba', 'zu': 'Zulu'}
        if language in language_dict:
            return language_dict[language]
        else:
            return ""
