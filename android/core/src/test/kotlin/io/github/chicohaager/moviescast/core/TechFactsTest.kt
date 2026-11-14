package io.github.chicohaager.moviescast.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** The line of facts on a film's page and the file line at its foot. */
class TechFactsTest {
    @Test
    fun anMkvFromMoviesIsCompletedFromItsHeader() {
        // as Movies answered for an MKV on 2026-11-14: codecs only; size and running time give the bitrate
        val server = TechFacts.of(
            MediaItem(id = "m", name = "M", type = "Movie", runtimeSeconds = 6004, sizeBytes = 56_900_000_000, videoCodec = "hevc",
                audio = listOf(Track(1, "truehd", "", "", 0))),
        )
        assertTrue(server.incomplete)
        assertEquals(75_816_122L, server.bitrate)
        val header = TechFacts(width = 3840, height = 2160, hdr = TechFacts.Hdr.DOLBY_VISION, audioCodec = "truehd", channels = 8, frameRate = 23.976f)
        val facts = server.mergedWith(header)
        assertFalse(facts.incomplete)
        assertEquals("4K", facts.resolution)
        assertEquals("Dolby Vision", facts.hdrLabel)
        assertEquals("TrueHD 7.1", facts.audioLabel)
        assertEquals("23.976 fps", facts.frameRateLabel)
        assertEquals("76 Mbit/s", facts.bitrateLabel)
    }

    @Test
    fun theServersValueWinsWhereItHasOne() {
        val server = TechFacts(width = 1920, height = 1080, audioCodec = "aac", channels = 2, bitrate = 12_712_949)
        val header = TechFacts(width = 3840, height = 2160, hdr = TechFacts.Hdr.SDR, audioCodec = "ac3", channels = 6, bitrate = 1)
        val facts = server.mergedWith(header)
        assertEquals(1920, facts.width)
        assertEquals("AAC 2.0", facts.audioLabel)
        assertEquals(TechFacts.Hdr.SDR, facts.hdr)
        assertEquals("12.7 Mbit/s", facts.bitrateLabel)
    }

    @Test
    fun jellyfinsVideoRangeTypes() {
        fun hdr(range: String) = TechFacts.of(MediaItem(id = "x", name = "X", type = "Movie", videoRange = range)).hdr
        assertEquals(TechFacts.Hdr.UNKNOWN, hdr(""))
        assertEquals(TechFacts.Hdr.UNKNOWN, hdr("Unknown"))
        assertEquals(TechFacts.Hdr.SDR, hdr("SDR"))
        assertEquals(TechFacts.Hdr.HDR10, hdr("HDR10"))
        assertEquals(TechFacts.Hdr.HDR10, hdr("HDR10Plus"))
        assertEquals(TechFacts.Hdr.HLG, hdr("HLG"))
        assertEquals(TechFacts.Hdr.DOLBY_VISION, hdr("DOVI"))
        assertEquals(TechFacts.Hdr.DOLBY_VISION, hdr("DOVIWithHDR10"))
    }

    @Test
    fun labels() {
        assertEquals("1080p", TechFacts(width = 2046, height = 852).resolution) // a scope film
        assertEquals("720p", TechFacts(width = 1280, height = 720).resolution)
        assertEquals("SD", TechFacts(width = 720, height = 576).resolution)
        assertNull(TechFacts().resolution)
        assertNull(TechFacts(hdr = TechFacts.Hdr.SDR).hdrLabel)
        assertEquals("HDR", TechFacts(hdr = TechFacts.Hdr.HDR10).hdrLabel)
        assertEquals("Dolby Digital+ 5.1", TechFacts(audioCodec = "eac3", channels = 6).audioLabel)
        assertEquals("DTS", TechFacts(audioCodec = "dts").audioLabel)
        assertEquals("VORBIS 2.0", TechFacts(audioCodec = "vorbis", channels = 2).audioLabel)
        assertNull(TechFacts().audioLabel)
        assertEquals("25 fps", TechFacts(frameRate = 25f).frameRateLabel)
        assertEquals("800 kbit/s", TechFacts(bitrate = 800_000).bitrateLabel)
    }
}
