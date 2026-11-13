package io.github.chicohaager.moviescast.core

import java.net.Socket
import java.security.KeyStore
import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.net.ssl.SSLEngine
import javax.net.ssl.SSLSocket
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509ExtendedTrustManager
import javax.net.ssl.X509TrustManager

/** The certificate of a server was not the one it was logged in with; logging in again binds the new one. */
class CertificateNotOfLoginException(val server: String) : CertificateException("the certificate of $server is not the one of the login")

/**
 * Which certificates count for which server (the owner's decision, 2026-11-13: a certificate the person
 * installed in Android's settings counts only for their own servers, and only the one it was logged in with).
 *
 * - A server of a login bound to [SYSTEM]: the system's certificates only; no user certificate ever.
 * - A server of a login bound to a user certificate (`user:<sha-256 of it>`): that certificate only. Any
 *   other chain, also one the system trusts, fails with [CertificateNotOfLoginException].
 * - The server being logged in to ([pending]) and a login not bound yet (made before there was a
 *   binding): the system's certificates, else one of the user certificates; [onBound] is told which.
 * - Every other server: the system's certificates only.
 *
 * Servers are named `host:port`. A user certificate counts when the platform's own check (PKIX) accepts the
 * chain with that certificate as its only root. They are read on each handshake, so a certificate
 * installed or removed counts at once. The name in the certificate is checked afterwards by OkHttp.
 */
class ScopedTrust(
    private val system: X509TrustManager,
    private val userCertificates: () -> List<X509Certificate>,
    /** The binding of the logins at a server: [SYSTEM], `user:<sha-256>`, "" (a login not bound yet) or null (no login). */
    private val binding: (String) -> String?,
    /** The servers being logged in to now (the candidates of the address typed); empty most of the time. */
    private val pending: () -> Set<String>,
    private val onBound: (server: String, binding: String) -> Unit,
) : X509ExtendedTrustManager() {

    override fun checkServerTrusted(chain: Array<X509Certificate>, authType: String, socket: Socket?) =
        check(chain, authType, (socket as? SSLSocket)?.handshakeSession?.let { "${it.peerHost}:${it.peerPort}" } ?: socket?.let { "${it.inetAddress?.hostName}:${it.port}" })

    override fun checkServerTrusted(chain: Array<X509Certificate>, authType: String, engine: SSLEngine?) =
        check(chain, authType, engine?.let { "${it.peerHost}:${it.peerPort}" })

    override fun checkServerTrusted(chain: Array<X509Certificate>, authType: String) = check(chain, authType, null)

    private fun check(chain: Array<X509Certificate>, authType: String, server: String?) {
        if (chain.isEmpty()) throw CertificateException("no certificate")
        val bound = server?.let(binding)
        val asking = server != null && server in pending()
        when {
            // being logged in to (again): as for a new server, and the binding is made anew
            asking || bound == "" -> {
                val how = systemOrUser(chain, authType)
                if (server != null) onBound(server, how)
            }
            bound == SYSTEM || bound == null -> system.checkServerTrusted(chain, authType)
            else -> {
                val ca = userCertificates().firstOrNull { userBinding(it) == bound } ?: throw CertificateNotOfLoginException(server!!)
                if (!rootedIn(chain, authType, ca)) throw CertificateNotOfLoginException(server!!)
            }
        }
    }

    /** [SYSTEM] when the system trusts the chain, else the binding of the user certificate that issued it. */
    private fun systemOrUser(chain: Array<X509Certificate>, authType: String): String {
        try {
            system.checkServerTrusted(chain, authType)
            return SYSTEM
        } catch (refused: CertificateException) {
            val ca = userCertificates().firstOrNull { rootedIn(chain, authType, it) } ?: throw refused
            return userBinding(ca)
        }
    }

    /**
     * What a login with [chain] (the certificates of its answer) is bound to: [SYSTEM] when the system
     * trusts it or there was no TLS, else the user certificate it is rooted in. Null: neither, which
     * cannot be after a login the check let through.
     */
    fun bindingOf(chain: List<X509Certificate>): String? {
        if (chain.isEmpty()) return SYSTEM
        val array = chain.toTypedArray()
        try {
            system.checkServerTrusted(array, AUTH)
            return SYSTEM
        } catch (_: CertificateException) {
            return userCertificates().firstOrNull { rootedIn(array, AUTH, it) }?.let(::userBinding)
        }
    }

    override fun checkClientTrusted(chain: Array<X509Certificate>, authType: String, socket: Socket?) = throw CertificateException("no client certificates")
    override fun checkClientTrusted(chain: Array<X509Certificate>, authType: String, engine: SSLEngine?) = throw CertificateException("no client certificates")
    override fun checkClientTrusted(chain: Array<X509Certificate>, authType: String) = throw CertificateException("no client certificates")
    override fun getAcceptedIssuers(): Array<X509Certificate> = system.acceptedIssuers + userCertificates()

    companion object {
        const val SYSTEM = "system"

        /** The key exchange is not known after the fact; TLS 1.3 names none either. */
        private const val AUTH = "UNKNOWN"

        fun userBinding(ca: X509Certificate): String = "user:" + sha256(ca.encoded)

        fun sha256(bytes: ByteArray): String = MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }

        /** Does the platform's check (PKIX) accept [chain] with [ca] as its only root? */
        internal fun rootedIn(chain: Array<X509Certificate>, authType: String, ca: X509Certificate): Boolean {
            val store = KeyStore.getInstance(KeyStore.getDefaultType()).apply {
                load(null, null)
                setCertificateEntry("ca", ca)
            }
            val factory = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(store) }
            val only = factory.trustManagers.filterIsInstance<X509TrustManager>().single()
            return try {
                only.checkServerTrusted(chain, authType)
                true
            } catch (_: CertificateException) {
                false
            }
        }
    }
}
