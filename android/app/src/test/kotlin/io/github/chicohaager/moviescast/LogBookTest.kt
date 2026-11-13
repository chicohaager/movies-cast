package io.github.chicohaager.moviescast

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

/** Invented values only: the addresses are documentation addresses (RFC 5737) and example hosts. */
class LogBookTest {
    @Test
    fun keepsTheLastLinesOnly() {
        val book = LogBook(capacity = 3)
        for (i in 1..5) book.add("line $i")
        assertEquals(listOf("line 3", "line 4", "line 5"), book.lines())
    }

    @Test
    fun anAddressNeverGetsIntoTheBook() {
        val book = LogBook()
        book.add("open http://192.0.2.10:8096/movies/Videos/abc/stream?static=true failed")
        book.add("host 192.0.2.10 did not answer, nor 192.0.2.10:8096")
        book.add("https://nas.example/movies/master.m3u8?api_key=invented-key")
        val all = book.lines().joinToString("\n")
        assertFalse(all, all.contains("192.0.2"))
        assertFalse(all, all.contains("nas.example"))
        assertFalse(all, all.contains("invented-key"))
        assertEquals("open <address> failed", book.lines()[0])
        assertEquals("host <address> did not answer, nor <address>", book.lines()[1])
    }

    @Test
    fun versionsAndFormatsStayReadable() {
        // Three-part versions and decoder names are what a report is read for.
        for (line in listOf("audio decoder ffmpegLavc60.3.100-truehd", "server Jellyfin 10.11.0", "video format video/hevc 3840x2160", "first frame 412 ms after opening")) {
            assertEquals(line, LogBook.scrub(line))
        }
    }

    @Test
    fun theReportListsFactsThenLinesAndScrubsBoth() {
        val text = LogBook.report(listOf("app" to "0.1.0 (100) release", "server" to "seen at 192.0.2.10"), listOf("playback error ERROR_CODE_IO_NETWORK_CONNECTION_FAILED", "retry http://192.0.2.10/x"))
        assertEquals(
            "app: 0.1.0 (100) release\nserver: seen at <address>\n\nplayer log (2 lines)\nplayback error ERROR_CODE_IO_NETWORK_CONNECTION_FAILED\nretry <address>\n",
            text,
        )
    }

    @Test
    fun theDecodersComeAfterTheLog() {
        val text = LogBook.report(listOf("app" to "0.1.0"), listOf("first frame 412 ms after opening"), listOf("c2.android.aac.decoder: audio/mp4a-latm", "c2.exynos.hevc.decoder: video/hevc"))
        assertEquals(
            "app: 0.1.0\n\nplayer log (1 lines)\nfirst frame 412 ms after opening\n\ndecoders (2)\nc2.android.aac.decoder: audio/mp4a-latm\nc2.exynos.hevc.decoder: video/hevc\n",
            text,
        )
    }
}
