package io.github.chicohaager.moviescast.core

import java.io.IOException
import java.net.InetAddress
import java.security.cert.X509Certificate
import javax.net.ssl.SSLContext
import kotlinx.coroutines.runBlocking
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import okhttp3.Dns
import okhttp3.OkHttpClient
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

/**
 * Which certificate counts for which server. "The system" trusts one authority made here (as Android
 * trusts the public ones); the person installed one of their own; a stranger installed another.
 * Invented names only.
 */
class ScopedTrustTest {
    private val server = MockWebServer()
    private val device = Device("dev-1", "Test phone", "0.0.1")
    private val public = HeldCertificate.Builder().certificateAuthority(0).commonName("invented public authority").build()
    private val own = HeldCertificate.Builder().certificateAuthority(0).commonName("invented own authority").build()
    private val stranger = HeldCertificate.Builder().certificateAuthority(0).commonName("invented stranger").build()

    private var installed: List<X509Certificate> = listOf(own.certificate, stranger.certificate)
    private val bindings = HashMap<String, String>()
    private var pending: Set<String> = emptySet()
    private val bound = ArrayList<Pair<String, String>>()

    private val systemTrust = HandshakeCertificates.Builder().addTrustedCertificate(public.certificate).build().trustManager
    private val trust = ScopedTrust(systemTrust, { installed }, { bindings[it] }, { pending }, { s, b -> bound.add(s to b) })

    private val http: OkHttpClient = SSLContext.getInstance("TLS").let { context ->
        context.init(null, arrayOf(trust), null)
        OkHttpClient.Builder()
            .dns { host -> if (host == "localhost") listOf(InetAddress.getByName("127.0.0.1")) else Dns.SYSTEM.lookup(host) }
            .sslSocketFactory(context.socketFactory, trust)
            .build()
    }
    private val client = MoviesClient(http, device)
    private val root get() = "https://localhost:${server.port}"
    private val name get() = "localhost:${server.port}"

    /** The server shows a certificate for "localhost" issued by [by]. */
    private fun serve(by: HeldCertificate) {
        val leaf = HeldCertificate.Builder().addSubjectAlternativeName("localhost").signedBy(by).build()
        server.useHttps(HandshakeCertificates.Builder().heldCertificate(leaf, by.certificate).build().sslSocketFactory())
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val body = if (request.url.encodedPath == "/movies/Users/AuthenticateByName") """{"AccessToken":"invented-token","User":{"Id":"u1","Name":"demo"}}"""
                else """{"ProductName":"Fake Movies","ServerName":"fake","Version":"10.11.0"}"""
                return MockResponse.Builder().code(200).addHeader("Content-Type", "application/json").body(body).build()
            }
        }
        server.start(InetAddress.getByName("127.0.0.1"), 0)
    }

    private fun answers() = runBlocking { client.publicInfo("$root/movies").version == "10.11.0" }

    /** Logs in (the server is [pending] meanwhile) and answers what the login is bound to. */
    private fun loginBinding(): String? {
        pending = setOf(name)
        val chain = ArrayList<X509Certificate>()
        runBlocking { client.login("$root/movies", "demo", "invented-pw", chain) }
        pending = emptySet()
        return trust.bindingOf(chain)
    }

    private inline fun <reified T : IOException> refusedWith() {
        try {
            runBlocking { client.publicInfo("$root/movies") }
            fail("expected ${T::class.simpleName}")
        } catch (e: IOException) {
            assertTrue(e.toString(), e is T)
        }
    }

    @After
    fun stop() = server.close()

    @Test
    fun anInstalledCertificateDoesNotCountForAServerWithoutALogin() {
        serve(own)
        refusedWith<UntrustedCertificateException>()
        assertEquals(emptyList<Pair<String, String>>(), bound)
    }

    @Test
    fun whileLoggingInItCountsAndTheLoginIsBoundToIt() {
        serve(own)
        assertEquals(ScopedTrust.userBinding(own.certificate), loginBinding())
    }

    @Test
    fun aServerBoundToTheOwnCertificateIsReached() {
        serve(own)
        bindings[name] = ScopedTrust.userBinding(own.certificate)
        assertTrue(answers())
    }

    @Test
    fun aServerBoundToTheOwnCertificateRefusesTheStrangersAlsoInstalled() {
        serve(stranger)
        bindings[name] = ScopedTrust.userBinding(own.certificate)
        refusedWith<CertificateChangedException>()
    }

    @Test
    fun aServerBoundToTheOwnCertificateRefusesEvenOneTheSystemTrusts() {
        serve(public)
        bindings[name] = ScopedTrust.userBinding(own.certificate)
        refusedWith<CertificateChangedException>()
    }

    @Test
    fun aServerBoundToTheOwnCertificateIsRefusedOnceItWasRemoved() {
        serve(own)
        bindings[name] = ScopedTrust.userBinding(own.certificate)
        installed = listOf(stranger.certificate)
        refusedWith<CertificateChangedException>()
    }

    @Test
    fun aServerBoundToTheSystemNeverTakesAUserCertificate() {
        serve(own)
        bindings[name] = ScopedTrust.SYSTEM
        try {
            runBlocking { client.publicInfo("$root/movies") }
            fail("expected UntrustedCertificateException")
        } catch (e: IOException) {
            // "not trusted", not "changed": the system's check is the one that refused
            assertTrue(e.toString(), e is UntrustedCertificateException && e !is CertificateChangedException)
        }
    }

    @Test
    fun aServerWithACertificateTheSystemTrustsIsReachedAndBoundToTheSystem() {
        serve(public)
        assertTrue(answers())
        // the connection (and the TLS session) of the request before is used again: no handshake,
        // so the binding comes from the answer's certificates and not from the check
        assertEquals(ScopedTrust.SYSTEM, loginBinding())
    }

    @Test
    fun aLoginFromBeforeTheBindingIsBoundAtItsFirstConnection() {
        serve(own)
        bindings[name] = ""
        assertTrue(answers())
        assertEquals(listOf(name to ScopedTrust.userBinding(own.certificate)), bound.distinct())
    }

    @Test
    fun whileLoggingInACertificateNobodyInstalledIsRefused() {
        serve(own)
        installed = listOf(stranger.certificate)
        try {
            loginBinding()
            fail("expected UntrustedCertificateException")
        } catch (e: IOException) {
            assertTrue(e.toString(), e is UntrustedCertificateException)
        }
    }

    @Test
    fun loggingInAgainBindsTheNewCertificate() {
        serve(stranger)
        bindings[name] = ScopedTrust.userBinding(own.certificate)
        assertEquals(ScopedTrust.userBinding(stranger.certificate), loginBinding())
    }

    @Test
    fun overPlainHttpALoginIsBoundToTheSystem() {
        assertEquals(ScopedTrust.SYSTEM, trust.bindingOf(emptyList()))
    }

    @Test
    fun aChainThatOnlyNamesTheCertificateButIsSignedByAnotherDoesNotCount() {
        // the server's certificate carries the own authority's name as issuer, but the stranger signed it
        val forged = HeldCertificate.Builder().certificateAuthority(0).commonName("invented own authority").build()
        serve(forged)
        bindings[name] = ScopedTrust.userBinding(own.certificate)
        refusedWith<CertificateChangedException>()
    }
}
