# Secrets with SOPS

Secrets for deployments are stored here as encrypted YAML files managed by [Mozilla SOPS](https://github.com/mozilla/sops).

## Usage

1. Сгенерируйте или импортируйте Age-пару (`age-keygen -o ~/.config/sops/age/keys.txt`).
2. Пропишите путь до файла в переменной `SOPS_AGE_KEY_FILE` (например, добавьте в `~/.bashrc`).
3. Убедитесь, что ваш публичный ключ (`age1...`) присутствует в `.sops.yaml` (файл уже содержит ключ для demo среды; добавьте дополнительные при необходимости).
4. Создайте или отредактируйте секрет:

   ```bash
   sops --age <your-age-recipient> --encrypt deploy/secrets/qazna.enc.yaml > deploy/secrets/qazna.enc.yaml
   ```

   The file structure should match a Kubernetes Secret (using `data` or `stringData`).

5. При необходимости расшифруйте:

   ```bash
   sops --decrypt deploy/secrets/qazna.enc.yaml
   ```

6. Коммитьте только зашифрованные файлы. Локальные `.env` и расшифрованные копии не добавляйте в git.

## Tips

- Use `SOPS_AGE_KEY_FILE=~/.config/sops/age/keys.txt` to simplify commands.
- CI/CD runners should be configured with read-only access to the Age private key for decrypting at deploy time.
