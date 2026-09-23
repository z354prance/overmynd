# Unraid template

This template pre-fills the existing configuration path and dashboard port. It does not download, replace, or publish your private application settings. It uses the locally built `overmynd:auth-completion` image; no registry image is published by this template.

## Install from the existing checkout

Run in the Unraid terminal:

```bash
cd /mnt/user/appdata/overmynd &&
git pull --ff-only origin auth-completion &&
docker build -t overmynd:auth-completion . &&
mkdir -p /boot/config/plugins/dockerMan/templates-user &&
cp -n unraid/overmynd.xml /boot/config/plugins/dockerMan/templates-user/my-overmynd.xml
```

The copy does not overwrite an existing user template, which may contain customized ports or paths. If you already created Overmynd through Unraid, keep managing that existing container through Edit.

For a first migration from the command-line container, after the build and template installation succeed:

```bash
docker stop overmynd-dev &&
docker rm overmynd-dev
```

This removes only the old container, leaving the image and configuration directory in place. In **Docker → Add Container**, select **overmynd** from the user templates. Confirm these values, click **Apply**, and enable **Autostart** on the Docker page:

- Repository: `overmynd:auth-completion`
- Host port `18080` → container port `8080` (TCP)
- Host path `/mnt/user/appdata/overmynd/config` → container path `/config` (read/write)

The WebUI remains at `http://SERVER-IP:18080`. Do not run the old and new containers simultaneously against the same configuration directory.

## Updates

Pull and rebuild the image in the same checkout. Then recreate the container through Unraid using its saved template. The template uses a local image, so registry-based Update/Force Update is not the image build mechanism. Do not delete the image or the appdata directory. Keep the configuration mapping unchanged to preserve the database and administrator account.

Unraid stores user templates under `/boot/config/plugins/dockerMan/templates-user`; see the [Unraid documentation](https://docs.unraid.net/community-applications/). Adding this file to the repository does not automatically list the app in Community Applications.

## Icon for an existing container

The template includes the project icon. If you installed an earlier template, open the existing container's **Edit → Advanced View**, set **Icon URL** to the following, and click **Apply**:

```text
https://raw.githubusercontent.com/z354prance/overmynd/auth-completion/unraid/overmynd-icon.png
```

Refresh the Docker page afterward. You do not need to rebuild the application image or replace your saved configuration to add the icon.
