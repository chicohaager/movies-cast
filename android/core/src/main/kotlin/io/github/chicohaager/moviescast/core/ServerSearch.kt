package io.github.chicohaager.moviescast.core

import java.io.IOException
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.Inet4Address
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.NetworkInterface
import java.net.SocketException
import java.net.SocketTimeoutException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

/** A server that answered at [address] when the home network was searched. */
data class FoundServer(val address: String, val name: String, val product: String, val version: String)

/**
 * Finding servers in the home network without typing an address.
 *
 * A Jellyfin server answers the text [QUESTION] sent to UDP port [JELLYFIN_PORT] with a JSON object
 * that names its address (Jellyfin 10.11.0, measured 2026-11-13). IceWhale Movies 0.4.5 does not
 * answer it; a ZimaOS box announces itself as `_zimaos._tcp` by mDNS instead, and Movies is behind
 * its gateway at `/movies` if it is installed. The app asks both ways and keeps what then answers
 * `System/Info/Public` ([MoviesClient.confirm]).
 */
object ServerSearch {
    const val JELLYFIN_PORT = 7359
    const val QUESTION = "who is JellyfinServer?"
    const val ZIMAOS_SERVICE = "_zimaos._tcp"

    private val json = Json { ignoreUnknownKeys = true }

    /**
     * Base URLs to try for one answer to [QUESTION] that came from [sender]: the address the
     * server names, then the sender's own address with the same port. A Jellyfin in a container
     * names the address inside the container's own network (so did the test server), which
     * no phone reaches. Empty when the answer is not Jellyfin's.
     */
    fun beaconCandidates(answer: String, sender: String): List<String> {
        val named = try {
            json.parseToJsonElement(answer).jsonObject["Address"]?.jsonPrimitive?.content
        } catch (_: SerializationException) {
            null
        } catch (_: IllegalArgumentException) {
            null
        } ?: return emptyList()
        val url = named.toHttpUrlOrNull() ?: return emptyList()
        val fromSender = try {
            url.newBuilder().host(sender).build()
        } catch (_: IllegalArgumentException) {
            null
        }
        return listOfNotNull(url, fromSender).map { it.toString().trimEnd('/') }.distinct()
    }

    /** The base URL of Movies on a ZimaOS box that announced [host] and [port]. */
    fun zimaCandidate(host: String, port: Int): String =
        if (port == 80 || port <= 0) "http://$host/movies" else "http://$host:$port/movies"

    /**
     * The address of a box to put into a web address: its IPv4 one. Android's service search at
     * times names only an IPv6 link-local address first (Pixel 7, Android 17, 2026-11-13), which
     * does not fit into a web address; null then, and the box is asked again.
     */
    fun ipv4(addresses: List<InetAddress>): String? = addresses.firstOrNull { it is Inet4Address }?.hostAddress

    /** Where [QUESTION] goes: the broadcast address of every IPv4 network of this device, and the limited broadcast. */
    fun broadcastTargets(): List<InetSocketAddress> {
        val out = LinkedHashSet<InetAddress>()
        try {
            for (nic in NetworkInterface.getNetworkInterfaces()?.toList().orEmpty()) {
                if (!nic.isUp || nic.isLoopback) continue
                nic.interfaceAddresses.mapNotNullTo(out) { it.broadcast }
            }
        } catch (_: SocketException) {
            // no list of interfaces: the limited broadcast below is all there is
        }
        out.add(InetAddress.getByAddress(byteArrayOf(-1, -1, -1, -1)))
        return out.filterIsInstance<Inet4Address>().map { InetSocketAddress(it, JELLYFIN_PORT) }
    }

    /**
     * Sends [QUESTION] to [targets] and collects answers for [waitMs]. One list of base URLs per
     * server that answered ([beaconCandidates]). A target that cannot be sent to is left out.
     */
    suspend fun askJellyfin(targets: List<InetSocketAddress>, waitMs: Long): List<List<String>> =
        withContext(Dispatchers.IO) {
            val found = LinkedHashSet<List<String>>()
            DatagramSocket().use { socket ->
                socket.broadcast = true
                val question = QUESTION.toByteArray()
                var sent = 0
                for (target in targets) {
                    try {
                        socket.send(DatagramPacket(question, question.size, target))
                        sent++
                    } catch (_: IOException) {
                        // a network without a route for its broadcast
                    }
                }
                if (sent == 0) return@use
                val end = System.nanoTime() + waitMs * 1_000_000
                val buffer = ByteArray(4096)
                while (true) {
                    val left = (end - System.nanoTime()) / 1_000_000
                    if (left <= 0) break
                    socket.soTimeout = left.toInt().coerceAtLeast(1)
                    val packet = DatagramPacket(buffer, buffer.size)
                    try {
                        socket.receive(packet)
                    } catch (_: SocketTimeoutException) {
                        break
                    }
                    val candidates = beaconCandidates(String(packet.data, 0, packet.length), packet.address.hostAddress.orEmpty())
                    if (candidates.isNotEmpty()) found.add(candidates)
                }
            }
            found.toList()
        }
}
