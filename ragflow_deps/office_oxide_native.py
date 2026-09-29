import os
import tarfile

OFFICE_OXIDE_ARCHIVE = "office_oxide-v0.1.12-linux-x86_64.tar.gz"


def extract_office_oxide(archive_path, target):
    os.makedirs(target, exist_ok=True)
    with tarfile.open(archive_path) as tf:
        tf.extractall(target)
