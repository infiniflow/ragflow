import base64
import os
import sys
from Cryptodome.PublicKey import RSA
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from api.utils import decrypt, file_utils


def crypt(line):
    file_path = os.path.join(
        file_utils.get_project_base_directory(),
        "conf",
        "public.pem")
    rsa_key = RSA.importKey(open(file_path).read())
    cipher = Cipher_pkcs1_v1_5.new(rsa_key)
    return base64.b64encode(cipher.encrypt(
        line.encode('utf-8'))).decode("utf-8")


if __name__ == "__main__":
    pswd = crypt(sys.argv[1])
    print(pswd)
    print(decrypt(pswd))
