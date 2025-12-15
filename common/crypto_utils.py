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

import os
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
from cryptography.hazmat.primitives import padding
from cryptography.hazmat.backends import default_backend
from cryptography.hazmat.primitives.kdf.pbkdf2 import PBKDF2HMAC
from cryptography.hazmat.primitives import hashes


class BaseCrypto:
    """Base class for cryptographic algorithms"""
    
    # Magic header to identify encrypted data
    ENCRYPTED_MAGIC = b'RAGF'
    
    def __init__(self, key, iv=None, block_size=16, key_length=32, iv_length=16):
        """
        Initialize cryptographic algorithm
        
        Args:
            key: Encryption key
            iv: Initialization vector, automatically generated if None
            block_size: Block size
            key_length: Key length
            iv_length: Initialization vector length
        """
        self.block_size = block_size
        self.key_length = key_length
        self.iv_length = iv_length
        
        # Normalize key
        self.key = self._normalize_key(key)
        self.iv = iv
    
    def _normalize_key(self, key):
        """Normalize key length"""
        if isinstance(key, str):
            key = key.encode('utf-8')
        
        # Use PBKDF2 for key derivation to ensure correct key length
        kdf = PBKDF2HMAC(
            algorithm=hashes.SHA256(),
            length=self.key_length,
            salt=b"ragflow_crypto_salt",  # Fixed salt to ensure consistent key derivation results
            iterations=100000,
            backend=default_backend()
        )
        
        return kdf.derive(key)
    
    def encrypt(self, data):
        """
        Encrypt data (template method)
        
        Args:
            data: Data to encrypt (bytes)
            
        Returns:
            Encrypted data (bytes), format: magic_header + iv + encrypted_data
        """
        # Generate random IV
        iv = os.urandom(self.iv_length) if not self.iv else self.iv
        
        # Use PKCS7 padding
        padder = padding.PKCS7(self.block_size * 8).padder()
        padded_data = padder.update(data) + padder.finalize()
        
        # Delegate to subclass for specific encryption
        ciphertext = self._encrypt(padded_data, iv)
        
        # Return Magic Header + IV + encrypted data
        return self.ENCRYPTED_MAGIC + iv + ciphertext
    
    def decrypt(self, encrypted_data):
        """
        Decrypt data (template method)
        
        Args:
            encrypted_data: Encrypted data (bytes)
            
        Returns:
            Decrypted data (bytes)
        """
        # Check if data is encrypted by magic header
        if not encrypted_data.startswith(self.ENCRYPTED_MAGIC):
            # Not encrypted, return as-is
            return encrypted_data
        
        # Remove magic header
        encrypted_data = encrypted_data[len(self.ENCRYPTED_MAGIC):]
        
        # Separate IV and encrypted data
        iv = encrypted_data[:self.iv_length]
        ciphertext = encrypted_data[self.iv_length:]
        
        # Delegate to subclass for specific decryption
        padded_data = self._decrypt(ciphertext, iv)
        
        # Remove padding
        unpadder = padding.PKCS7(self.block_size * 8).unpadder()
        data = unpadder.update(padded_data) + unpadder.finalize()
        
        return data
    
    def _encrypt(self, padded_data, iv):
        """
        Encrypt padded data with specific algorithm
        
        Args:
            padded_data: Padded data to encrypt
            iv: Initialization vector
            
        Returns:
            Encrypted data
        """
        raise NotImplementedError("_encrypt method must be implemented by subclass")
    
    def _decrypt(self, ciphertext, iv):
        """
        Decrypt ciphertext with specific algorithm
        
        Args:
            ciphertext: Ciphertext to decrypt
            iv: Initialization vector
            
        Returns:
            Decrypted padded data
        """
        raise NotImplementedError("_decrypt method must be implemented by subclass")


class AESCrypto(BaseCrypto):
    """Base class for AES cryptographic algorithm"""
    
    def __init__(self, key, iv=None, key_length=32):
        """
        Initialize AES cryptographic algorithm
        
        Args:
            key: Encryption key
            iv: Initialization vector, automatically generated if None
            key_length: Key length (16 for AES-128, 32 for AES-256)
        """
        super().__init__(key, iv, block_size=16, key_length=key_length, iv_length=16)

    def _encrypt(self, padded_data, iv):
        """AES encryption implementation"""
        # Create encryptor
        cipher = Cipher(
            algorithms.AES(self.key),
            modes.CBC(iv),
            backend=default_backend()
        )
        encryptor = cipher.encryptor()
        
        # Encrypt data
        return encryptor.update(padded_data) + encryptor.finalize()
    
    def _decrypt(self, ciphertext, iv):
        """AES decryption implementation"""
        # Create decryptor
        cipher = Cipher(
            algorithms.AES(self.key),
            modes.CBC(iv),
            backend=default_backend()
        )
        decryptor = cipher.decryptor()
        
        # Decrypt data
        return decryptor.update(ciphertext) + decryptor.finalize()


class AES128CBC(AESCrypto):
    """AES-128-CBC cryptographic algorithm"""
    
    def __init__(self, key, iv=None):
        """
        Initialize AES-128-CBC cryptographic algorithm
        
        Args:
            key: Encryption key
            iv: Initialization vector, automatically generated if None
        """
        super().__init__(key, iv, key_length=16)


class AES256CBC(AESCrypto):
    """AES-256-CBC cryptographic algorithm"""
    
    def __init__(self, key, iv=None):
        """
        Initialize AES-256-CBC cryptographic algorithm
        
        Args:
            key: Encryption key
            iv: Initialization vector, automatically generated if None
        """
        super().__init__(key, iv, key_length=32)


class SM4CBC(BaseCrypto):
    """SM4-CBC cryptographic algorithm using cryptography library for better performance"""
    
    def __init__(self, key, iv=None):
        """
        Initialize SM4-CBC cryptographic algorithm
        
        Args:
            key: Encryption key
            iv: Initialization vector, automatically generated if None
        """
        super().__init__(key, iv, block_size=16, key_length=16, iv_length=16)

    def _encrypt(self, padded_data, iv):
        """SM4 encryption implementation using cryptography library"""
        # Create encryptor
        cipher = Cipher(
            algorithms.SM4(self.key),
            modes.CBC(iv),
            backend=default_backend()
        )
        encryptor = cipher.encryptor()
        
        # Encrypt data
        return encryptor.update(padded_data) + encryptor.finalize()
    
    def _decrypt(self, ciphertext, iv):
        """SM4 decryption implementation using cryptography library"""
        # Create decryptor
        cipher = Cipher(
            algorithms.SM4(self.key),
            modes.CBC(iv),
            backend=default_backend()
        )
        decryptor = cipher.decryptor()
        
        # Decrypt data
        return decryptor.update(ciphertext) + decryptor.finalize()


class CryptoUtil:
    """Cryptographic utility class, using factory pattern to create cryptographic algorithm instances"""
    
    # Supported cryptographic algorithms mapping
    SUPPORTED_ALGORITHMS = {
        "aes-128-cbc": AES128CBC,
        "aes-256-cbc": AES256CBC,
        "sm4-cbc": SM4CBC
    }
    
    def __init__(self, algorithm="aes-256-cbc", key=None, iv=None):
        """
        Initialize cryptographic utility
        
        Args:
            algorithm: Cryptographic algorithm, default is aes-256-cbc
            key: Encryption key, uses RAGFLOW_CRYPTO_KEY environment variable if None
            iv: Initialization vector, automatically generated if None
        """
        if algorithm not in self.SUPPORTED_ALGORITHMS:
            raise ValueError(f"Unsupported algorithm: {algorithm}")
            
        if not key:
            raise ValueError("Encryption key not provided and RAGFLOW_CRYPTO_KEY environment variable not set")
        
        # Create cryptographic algorithm instance
        self.algorithm_name = algorithm
        self.crypto = self.SUPPORTED_ALGORITHMS[algorithm](key=key, iv=iv)
    
    def encrypt(self, data):
        """
        Encrypt data
        
        Args:
            data: Data to encrypt (bytes)
            
        Returns:
            Encrypted data (bytes)
        """
        # import time
        # start_time = time.time()
        encrypted = self.crypto.encrypt(data)
        # end_time = time.time()
        # logging.info(f"Encryption completed, data length: {len(data)} bytes, time: {(end_time - start_time)*1000:.2f} ms")
        return encrypted
    
    def decrypt(self, encrypted_data):
        """
        Decrypt data
        
        Args:
            encrypted_data: Encrypted data (bytes)
            
        Returns:
            Decrypted data (bytes)
        """
        # import time
        # start_time = time.time()
        decrypted = self.crypto.decrypt(encrypted_data)
        # end_time = time.time()
        # logging.info(f"Decryption completed, data length: {len(encrypted_data)} bytes, time: {(end_time - start_time)*1000:.2f} ms")
        return decrypted


# Test code
if __name__ == "__main__":
    # Test AES encryption
    crypto = CryptoUtil(algorithm="aes-256-cbc", key="test_key_123456")
    test_data = b"Hello, RAGFlow! This is a test for encryption."
    
    encrypted = crypto.encrypt(test_data)
    decrypted = crypto.decrypt(encrypted)
    
    print("AES Test:")
    print(f"Original: {test_data}")
    print(f"Encrypted: {encrypted}")
    print(f"Decrypted: {decrypted}")
    print(f"Success: {test_data == decrypted}")
    print()
    
    # Test SM4 encryption
    try:
        crypto_sm4 = CryptoUtil(algorithm="sm4-cbc", key="test_key_123456")
        encrypted_sm4 = crypto_sm4.encrypt(test_data)
        decrypted_sm4 = crypto_sm4.decrypt(encrypted_sm4)
        
        print("SM4 Test:")
        print(f"Original: {test_data}")
        print(f"Encrypted: {encrypted_sm4}")
        print(f"Decrypted: {decrypted_sm4}")
        print(f"Success: {test_data == decrypted_sm4}")
    except Exception as e:
        print(f"SM4 Test Failed: {e}")
        import traceback
        traceback.print_exc()
    
    # Test with specific algorithm classes directly
    print("\nDirect Algorithm Class Test:")
    
    # Test AES-128-CBC
    aes128 = AES128CBC(key="test_key_123456")
    encrypted_aes128 = aes128.encrypt(test_data)
    decrypted_aes128 = aes128.decrypt(encrypted_aes128)
    print(f"AES-128-CBC test: {'passed' if decrypted_aes128 == test_data else 'failed'}")
    
    # Test AES-256-CBC
    aes256 = AES256CBC(key="test_key_123456")
    encrypted_aes256 = aes256.encrypt(test_data)
    decrypted_aes256 = aes256.decrypt(encrypted_aes256)
    print(f"AES-256-CBC test: {'passed' if decrypted_aes256 == test_data else 'failed'}")
    
    # Test SM4-CBC
    try:
        sm4 = SM4CBC(key="test_key_123456")
        encrypted_sm4 = sm4.encrypt(test_data)
        decrypted_sm4 = sm4.decrypt(encrypted_sm4)
        print(f"SM4-CBC test: {'passed' if decrypted_sm4 == test_data else 'failed'}")
    except Exception as e:
        print(f"SM4-CBC test failed: {e}")
