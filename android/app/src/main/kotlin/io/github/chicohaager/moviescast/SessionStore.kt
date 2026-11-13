package io.github.chicohaager.moviescast

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import io.github.chicohaager.moviescast.core.SavedLogin
import io.github.chicohaager.moviescast.core.SavedLogins
import io.github.chicohaager.moviescast.core.Session
import java.security.GeneralSecurityException
import java.security.KeyStore
import java.util.UUID
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Keeps the logins across app starts: every server and account the person logged in to, and which
 * of them is in use. Each token is stored encrypted with an AES key that lives in the Android
 * keystore and never leaves the device; addresses and user names are stored in the clear so the
 * screens can show them.
 */
class SessionStore(context: Context, name: String = "session") {
    private val prefs = context.getSharedPreferences(name, Context.MODE_PRIVATE)

    /** Identifies this installation to the server (its device list), made once. */
    val deviceId: String
        get() = prefs.getString(KEY_DEVICE, null) ?: UUID.randomUUID().toString().also {
            prefs.edit().putString(KEY_DEVICE, it).apply()
        }

    val lastServer: String get() = prefs.getString(KEY_SERVER_TYPED, "").orEmpty()
    val lastUser: String get() = prefs.getString(KEY_USER_NAME, "").orEmpty()

    /** The order of the library chosen last ([io.github.chicohaager.moviescast.core.LibrarySort]'s name), or "". */
    var sort: String
        get() = prefs.getString(KEY_SORT, "").orEmpty()
        set(value) = prefs.edit().putString(KEY_SORT, value).apply()

    fun rememberTyped(server: String) {
        prefs.edit().putString(KEY_SERVER_TYPED, server).apply()
    }

    /** The logins kept, and which is in use. A login of the app before there was a list is taken over once. */
    fun logins(): SavedLogins {
        prefs.getString(KEY_LOGINS, null)?.let { return SavedLogins.decode(it) }
        val base = prefs.getString(KEY_BASE, null)
        val userId = prefs.getString(KEY_USER_ID, null)
        val sealed = prefs.getString(KEY_TOKEN, null)
        val taken = if (base != null && userId != null && sealed != null) SavedLogins().with(SavedLogin(base, userId, lastUser, sealed)) else SavedLogins()
        prefs.edit().putString(KEY_LOGINS, taken.encode()).remove(KEY_BASE).remove(KEY_USER_ID).remove(KEY_TOKEN).apply()
        return taken
    }

    private fun write(logins: SavedLogins) = prefs.edit().putString(KEY_LOGINS, logins.encode()).apply()

    /**
     * Keeps the login and puts it in use; [serverName] is what the server calls itself, [trust] what its
     * certificates must come from ([io.github.chicohaager.moviescast.core.ScopedTrust]); every login at the
     * same server gets that binding.
     */
    fun save(s: Session, serverName: String = "", trust: String = "") {
        val sealed = try {
            encrypt(s.token)
        } catch (_: GeneralSecurityException) {
            return // without a keystore the login lasts until the app is closed
        }
        val login = SavedLogin(s.baseUrl, s.userId, s.userName, sealed, serverName, trust)
        write(logins().with(login).bound(login.server, trust))
        prefs.edit().putString(KEY_USER_NAME, s.userName).apply()
    }

    /** The login in use, or null. */
    fun load(): Session? = logins().active?.let(::open)

    /** The login at [index] with its token, or null when the index does not exist or the token cannot be read. */
    fun session(index: Int): Session? = logins().logins.getOrNull(index)?.let(::open)

    /** Puts the login at [index] in use and returns it; null (and nothing changed) when it cannot be read. */
    fun choose(index: Int): Session? {
        val session = session(index) ?: return null
        write(logins().chosen(index))
        return session
    }

    /** The binding of the logins at [server] (`host:port`), null when there is none. */
    fun trustOf(server: String): String? = logins().trustOf(server)

    /** A login made before there was a binding is bound at its first connection. */
    fun bind(server: String, trust: String) {
        val all = logins()
        if (all.trustOf(server) == "") write(all.bound(server, trust))
    }

    /** Forgets the login at [index]. */
    fun remove(index: Int) = write(logins().without(index))

    /** Forgets the login in use; the others stay. */
    fun clear() {
        val all = logins()
        write(all.without(all.current))
    }

    /** Forgets every login. */
    fun clearAll() = write(SavedLogins())

    private fun open(login: SavedLogin): Session? {
        val token = try {
            decrypt(login.sealedToken)
        } catch (_: GeneralSecurityException) {
            return null
        } catch (_: IllegalArgumentException) {
            return null
        }
        return Session(login.baseUrl, token, login.userId, login.userName)
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
        const val KEY_SORT = "sort"
        const val KEY_LOGINS = "logins"
    }
}
