#[cfg(target_os = "macos")]
mod macos {
    use anyhow::{Context, Result};
    use security_framework::passwords::{delete_generic_password, get_generic_password, set_generic_password};

    const SERVICE_NAME: &str = "com.knowledgemesh.worker";

    /// Store a credential in macOS Keychain.
    #[allow(dead_code)]
    pub fn store(key_name: &str, value: &str) -> Result<()> {
        // Delete any existing entry first (set_generic_password fails if it exists)
        let _ = delete_generic_password(SERVICE_NAME, key_name);
        set_generic_password(SERVICE_NAME, key_name, value.as_bytes())
            .context("Failed to store credential in macOS Keychain")?;
        Ok(())
    }

    /// Retrieve a credential from macOS Keychain.
    pub fn retrieve(key_name: &str) -> Result<String> {
        let bytes = get_generic_password(SERVICE_NAME, key_name)
            .context("Credential not found in macOS Keychain — run `km-worker setup` first")?;
        String::from_utf8(bytes.to_vec()).context("Credential in Keychain is not valid UTF-8")
    }

    /// Delete a credential from macOS Keychain.
    #[allow(dead_code)]
    pub fn delete(key_name: &str) -> Result<()> {
        delete_generic_password(SERVICE_NAME, key_name)
            .context("Failed to delete credential from macOS Keychain")?;
        Ok(())
    }
}

#[cfg(not(target_os = "macos"))]
mod fallback {
    use anyhow::Result;

    #[allow(dead_code)]
    pub fn store(_key_name: &str, _value: &str) -> Result<()> {
        anyhow::bail!("Keychain storage is only available on macOS. Use environment variables instead.")
    }

    pub fn retrieve(_key_name: &str) -> Result<String> {
        anyhow::bail!("Keychain storage is only available on macOS. Use environment variables instead.")
    }

    #[allow(dead_code)]
    pub fn delete(_key_name: &str) -> Result<()> {
        anyhow::bail!("Keychain storage is only available on macOS. Use environment variables instead.")
    }
}

#[cfg(target_os = "macos")]
pub use macos::*;

#[cfg(not(target_os = "macos"))]
pub use fallback::*;
