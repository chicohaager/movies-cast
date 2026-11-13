package io.github.chicohaager.moviescast

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import io.github.chicohaager.moviescast.core.Session
import java.security.GeneralSecurityException
import java.security.KeyStore
import java.util.UUID
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Keeps the login across app starts. The token is stored encrypted with an AES key that lives in
 * the Android keystore and never leaves the device; address and user name are stored in the clear
 * so the login screen can offer them again.
 */
class SessionStore(context: Context) {
    private val prefs = context.getSharedPreferences("session", Context.MODE_PRIVATE)

    /** Identifies this installation to the server (its device list), made once. */
    val deviceId: String
        get() = prefs.getString(KEY_DEVICE, null) ?: UUID.randomUUID().toString().also {
            prefs.edit().putString(KEY_DEVICE, it).apply()
        }

    val lastServer: String get() = prefs.getString(KEY_SERVER_TYPED, "").orEmpty()
    val lastUser: String get() = prefs.getString(KEY_USER_NAME, "").orEmpty()

    fun rememberTyped(server: String) {
        prefs.edit().putString(KEY_SERVER_TYPED, server).apply()
    }

    fun save(s: Session) {
        val sealed = try {
            encrypt(s.token)
        } catch (_: GeneralSecurityException) {
            return // without a keystore the login lasts until the app is closed
        }
        prefs.edit()
            .putString(KEY_BASE, s.baseUrl)
            .putString(KEY_USER_ID, s.userId)
            .putString(KEY_USER_NAME, s.userName)
            .putString(KEY_TOKEN, sealed)
            .apply()
    }

    fun load(): Session? {
        val base = prefs.getString(KEY_BASE, null) ?: return null
        val userId = prefs.getString(KEY_USER_ID, null) ?: return null
        val sealed = prefs.getString(KEY_TOKEN, null) ?: return null
        val token = try {
            decrypt(sealed)
        } catch (_: GeneralSecurityException) {
            return null
        } catch (_: IllegalArgumentException) {
            return null
        }
        return Session(base, token, userId, lastUser)
    }

    fun clear() {
        prefs.edit().remove(KEY_BASE).remove(KEY_USER_ID).remove(KEY_TOKEN).apply()
    }

    private fun key(): SecretKey {
        val keyStore = KeyStore.getInstance(KEYSTORE).apply { load(null) }
        (keyStore.getKey(ALIAS, null) as? SecretKey)?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build(),
        )
        return generator.generateKey()
    }

    private fun encrypt(plain: String): String {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, key())
        val body = cipher.doFinal(plain.toByteArray(Charsets.UTF_8))
        return Base64.encodeToString(cipher.iv, Base64.NO_WRAP) + ":" + Base64.encodeToString(body, Base64.NO_WRAP)
    }

    private fun decrypt(sealed: String): String {
        val parts = sealed.split(":")
        require(parts.size == 2)
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, Base64.decode(parts[0], Base64.NO_WRAP)))
        return String(cipher.doFinal(Base64.decode(parts[1], Base64.NO_WRAP)), Charsets.UTF_8)
    }

    private companion object {
        const val KEYSTORE = "AndroidKeyStore"
        const val ALIAS = "movies-cast-session"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val KEY_DEVICE = "device_id"
        const val KEY_SERVER_TYPED = "server_typed"
        const val KEY_BASE = "base_url"
        const val KEY_USER_ID = "user_id"
        const val KEY_USER_NAME = "user_name"
        const val KEY_TOKEN = "token_sealed"
    }
}
