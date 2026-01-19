#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

import base64
import os
import sys
from Cryptodome.PublicKey import RSA
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from common.file_utils import get_project_base_directory


def crypt(line):
    """
    decrypt(crypt(input_string)) == base64(input_string), which frontend and admin_client use.
    """
    file_path = os.path.join(get_project_base_directory(), "conf", "public.pem")
    rsa_key = RSA.importKey(open(file_path).read(), "Welcome")
    cipher = Cipher_pkcs1_v1_5.new(rsa_key)
    password_base64 = base64.b64encode(line.encode('utf-8')).decode("utf-8")
    encrypted_password = cipher.encrypt(password_base64.encode())
    return base64.b64encode(encrypted_password).decode('utf-8')


def decrypt(line):
    file_path = os.path.join(get_project_base_directory(), "conf", "private.pem")
    rsa_key = RSA.importKey(open(file_path).read(), "Welcome")
    cipher = Cipher_pkcs1_v1_5.new(rsa_key)
    return cipher.decrypt(base64.b64decode(line), "Fail to decrypt password!").decode('utf-8')


def decrypt2(crypt_text):
    from base64 import b64decode, b16decode
    from Crypto.Cipher import PKCS1_v1_5 as Cipher_PKCS1_v1_5
    from Crypto.PublicKey import RSA
    decode_data = b64decode(crypt_text)
    if len(decode_data) == 127:
        hex_fixed = '00' + decode_data.hex()
        decode_data = b16decode(hex_fixed.upper())

    file_path = os.path.join(get_project_base_directory(), "conf", "private.pem")
    pem = open(file_path).read()
    rsa_key = RSA.importKey(pem, "Welcome")
    cipher = Cipher_PKCS1_v1_5.new(rsa_key)
    decrypt_text = cipher.decrypt(decode_data, None)
    return (b64decode(decrypt_text)).decode()


if __name__ == "__main__":
    passwd = crypt(sys.argv[1])
    print(passwd)
    print(decrypt(passwd))
