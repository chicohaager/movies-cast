package io.github.chicohaager.moviescast.core

import java.io.IOException
import java.net.ConnectException
import kotlinx.coroutines.runBlocking
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.OkHttpClient
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

/**
 * A server with a certificate it signed itself, made in the test. The device trusts it once the
 * owner installed it; until then the client says that the certificate is the obstacle.
 */
class CertificateTest {
    private val server = MockWebServer()
    private val device = Device("dev-1", "Test phone", "0.0.1")
    private val info = MockResponse.Builder().code(200).addHeader("Content-Type", "application/json")
        .body("""{"ProductName":"Fake Movies","ServerName":"fake","Version":"10.11.0"}""").build()

    /** By name, so the address is the one the certificate is (or is not) made out for. */
    private val root get() = "https://localhost:${server.port}"

    private fun serve(madeOutFor: String): HeldCertificate {
        val own = HeldCertificate.Builder().addSubjectAlternativeName(madeOutFor).build()
        server.useHttps(HandshakeCertificates.Builder().heldCertificate(own).build().sslSocketFactory())
        server.enqueue(info)
        server.enqueue(info)
        server.start()
        return own
    }

    private fun trusting(own: HeldCertificate): OkHttpClient {
        val trust = HandshakeCertificates.Builder().addTrustedCertificate(own.certificate).build()
        return OkHttpClient.Builder().sslSocketFactory(trust.sslSocketFactory(), trust.trustManager).build()
    }

    @After
    fun stop() = server.close()

    @Test
    fun aSelfSignedCertificateTheDeviceDoesNotKnowIsNamedAsTheReason() = runBlocking {
        serve("localhost")
        val client = MoviesClient(OkHttpClient(), device)
        try {
            client.publicInfo("$root/movies")
            fail("expected UntrustedCertificateException")
        } catch (e: IOException) {
            assertTrue(e.toString(), e is UntrustedCertificateException)
        }
        try {
            client.discover(root)
            fail("expected UntrustedCertificateException")
        } catch (e: IOException) {
            assertTrue(e.toString(), e is UntrustedCertificateException)
        }
    }

    @Test
    fun theSameCertificateOnceTrustedLetsTheServerAnswer() = runBlocking {
        val own = serve("localhost")
        val answer = MoviesClient(trusting(own), device).publicInfo("$root/movies")
        assertEquals(ServerInfo("Fake Movies", "fake", "10.11.0"), answer)
    }

    @Test
    fun aTrustedCertificateMadeOutForAnotherNameIsNamedAsTheReason() = runBlocking {
        val own = serve("other.example")
        try {
            MoviesClient(trusting(own), device).publicInfo("$root/movies")
            fail("expected UntrustedCertificateException")
        } catch (e: IOException) {
            assertTrue(e.toString(), e is UntrustedCertificateException)
        }
    }

    @Test
    fun ofSeveralErrorsTheCertificateIsTheReasonElseTheLast() {
        val refused = ConnectException("refused")
        val untrusted = UntrustedCertificateException()
        val none = IOException("no server info")
        assertSame(untrusted, MoviesClient.reason(listOf(none, untrusted, refused)))
        assertSame(refused, MoviesClient.reason(listOf(none, refused)))
        assertEquals("no address", MoviesClient.reason(emptyList()).message)
    }
}
