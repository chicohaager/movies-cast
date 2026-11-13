package io.github.chicohaager.moviescast.core

import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.Inet4Address
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.SocketException
import kotlin.concurrent.thread
import kotlinx.coroutines.runBlocking
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import okhttp3.OkHttpClient
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** Invented values only (RFC 5737 addresses); the sockets are on the loopback of the machine that runs the test. */
class ServerSearchTest {
    private val open = ArrayList<AutoCloseable>()

    @After
    fun close() = open.forEach { it.close() }

    /** Something on UDP that answers every packet with what [answer] makes of its text; null answers nothing. */
    private fun responder(answer: (String) -> String?): InetSocketAddress {
        val socket = DatagramSocket(0, InetAddress.getByName("127.0.0.1"))
        open.add(socket)
        thread(isDaemon = true) {
            val buffer = ByteArray(512)
            try {
                while (true) {
                    val packet = DatagramPacket(buffer, buffer.size)
                    socket.receive(packet)
                    val reply = answer(String(packet.data, 0, packet.length))?.toByteArray() ?: continue
                    socket.send(DatagramPacket(reply, reply.size, packet.socketAddress))
                }
            } catch (_: SocketException) {
                // closed by the test
            }
        }
        return InetSocketAddress(socket.localAddress, socket.localPort)
    }

    @Test
    fun anAnswerGivesTheNamedAddressAndTheSendersOwn() {
        // the form Jellyfin 10.11.0 answered with on 2026-11-13, with invented values
        val answer = """{"Address":"http://198.51.100.5:8096","Id":"0123456789abcdef","Name":"box","EndpointAddress":null}"""
        assertEquals(
            listOf("http://198.51.100.5:8096", "http://192.0.2.20:8096"),
            ServerSearch.beaconCandidates(answer, "192.0.2.20"),
        )
        assertEquals(listOf("http://192.0.2.20:8096"), ServerSearch.beaconCandidates("""{"Address":"http://192.0.2.20:8096/"}""", "192.0.2.20"))
        assertEquals(listOf("https://media.example/jf", "https://192.0.2.20/jf"), ServerSearch.beaconCandidates("""{"Address":"https://media.example/jf"}""", "192.0.2.20"))
    }

    @Test
    fun whatIsNotAnAnswerGivesNothing() {
        for (text in listOf("", "hello", "[1,2]", """{"Name":"box"}""", """{"Address":"not an address"}""", """{"Address":"ftp://192.0.2.1"}""", """{"Address":{"a":1}}""")) {
            assertEquals(text, emptyList<String>(), ServerSearch.beaconCandidates(text, "192.0.2.20"))
        }
    }

    @Test
    fun theQuestionIsAskedAndEveryServerThatAnswersIsCollected() = runBlocking {
        val asked = ArrayList<String>()
        val one = responder {
            synchronized(asked) { asked.add(it) }
            if (it == "who is JellyfinServer?") """{"Address":"http://198.51.100.5:8096","Name":"one"}""" else null
        }
        val noise = responder { "this is not a server" }
        val silent = responder { null }
        val found = ServerSearch.askJellyfin(listOf(one, noise, silent), 600)
        assertEquals(listOf(listOf("http://198.51.100.5:8096", "http://127.0.0.1:8096")), found)
        assertEquals(listOf("who is JellyfinServer?"), synchronized(asked) { asked.toList() })
    }

    @Test
    fun nobodyAnsweringEndsAfterTheWait() = runBlocking {
        val silent = responder { null }
        val begun = System.nanoTime()
        assertEquals(emptyList<List<String>>(), ServerSearch.askJellyfin(listOf(silent), 300))
        val took = (System.nanoTime() - begun) / 1_000_000
        assertTrue("took $took ms", took in 250..3000)
    }

    @Test
    fun theQuestionGoesToTheBroadcastAddressesOnJellyfinsPort() {
        val targets = ServerSearch.broadcastTargets()
        assertTrue(targets.toString(), targets.any { it.address.hostAddress == "255.255.255.255" })
        assertTrue(targets.all { it.port == 7359 && it.address is Inet4Address })
        assertEquals(targets.size, targets.distinct().size)
    }

    @Test
    fun ofTheAddressesOfABoxTheIpv4OneIsTaken() {
        val v6 = InetAddress.getByName("fe80::1")
        val v4 = InetAddress.getByName("192.0.2.10")
        assertEquals("192.0.2.10", ServerSearch.ipv4(listOf(v6, v4)))
        assertEquals(null, ServerSearch.ipv4(listOf(v6)))
        assertEquals(null, ServerSearch.ipv4(emptyList()))
    }

    @Test
    fun aZimaBoxIsAskedAtItsGateway() {
        assertEquals("http://192.0.2.10/movies", ServerSearch.zimaCandidate("192.0.2.10", 80))
        assertEquals("http://192.0.2.10:8080/movies", ServerSearch.zimaCandidate("192.0.2.10", 8080))
    }

    @Test
    fun onlyWhatAnswersTheInfoPageIsAServer() = runBlocking {
        val server = MockWebServer()
        open.add(server)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.url.encodedPath) {
                "/movies/System/Info/Public" -> MockResponse.Builder().code(200).addHeader("Content-Type", "application/json")
                    .body("""{"ProductName":"ZimaOS Movies","ServerName":"box","Version":"10.11.0"}""").build()
                "/empty/System/Info/Public" -> MockResponse.Builder().code(200).body("{}").build()
                else -> MockResponse.Builder().code(404).body("{}").build()
            }
        }
        server.start()
        val movies = server.url("/movies").toString()
        val client = MoviesClient(OkHttpClient(), Device("dev-1", "Test", "0.0.1"))
        val found = client.confirm(
            listOf(
                listOf("http://127.0.0.1:1/movies", movies), // the named address is dead, the second answers
                listOf(server.url("/other").toString()), // a box without Movies: 404
                listOf(server.url("/empty").toString()), // answers, but not as a server
                listOf(movies), // the same server found a second way
                listOf("not an address"),
            ),
            2000,
        )
        assertEquals(listOf(FoundServer(movies, "box", "ZimaOS Movies", "10.11.0")), found)
    }
}
