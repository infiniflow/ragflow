Place the Vastbase G100 license file(s) issued for your deployment in this
directory. `docker/docker-compose-base.yml` bind-mounts it into the container
at `/home/vastbase/vastbase/lic`:

    ./vastbase/license:/home/vastbase/vastbase/lic

The directory is committed with only this README (and a .gitkeep) so the
mount source always exists; license files themselves must never be committed.
