import tarfile

from office_oxide_native import OFFICE_OXIDE_ARCHIVE, extract_office_oxide


def test_upgrade_replaces_existing_native_library(tmp_path):
    assert "0.1.12" in OFFICE_OXIDE_ARCHIVE
    archive = tmp_path / OFFICE_OXIDE_ARCHIVE
    source = tmp_path / "source"
    (source / "lib").mkdir(parents=True)
    (source / "include" / "office_oxide_c").mkdir(parents=True)
    (source / "lib" / "liboffice_oxide.a").write_bytes(b"v0.1.12")
    (source / "include" / "office_oxide_c" / "office_oxide.h").write_bytes(b"new header")
    with tarfile.open(archive, "w:gz") as tf:
        tf.add(source / "lib", arcname="lib")
        tf.add(source / "include", arcname="include")

    target = tmp_path / "installed"
    (target / "lib").mkdir(parents=True)
    (target / "include" / "office_oxide_c").mkdir(parents=True)
    (target / "lib" / "liboffice_oxide.a").write_bytes(b"v0.1.11")
    (target / "include" / "office_oxide_c" / "office_oxide.h").write_bytes(b"old header")

    extract_office_oxide(archive, target)

    assert (target / "lib" / "liboffice_oxide.a").read_bytes() == b"v0.1.12"
    assert (target / "include" / "office_oxide_c" / "office_oxide.h").read_bytes() == b"new header"
