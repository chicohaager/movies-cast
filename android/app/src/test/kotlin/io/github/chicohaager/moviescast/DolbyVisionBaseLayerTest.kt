package io.github.chicohaager.moviescast

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class DolbyVisionBaseLayerTest {
    private val dv = "video/dolby-vision"

    @Test
    fun profile7WithoutADecoderPlaysAsHevc() {
        // The file measured on 2026-11-12: dvhe.07.06, HDR10 base layer.
        assertTrue(DolbyVisionBaseLayer.playAsHevc(dv, "dvhe.07.06", true, deviceHandlesTrack = false))
        assertTrue(DolbyVisionBaseLayer.playAsHevc(dv, "dvh1.07.09", null, deviceHandlesTrack = false))
    }

    @Test
    fun aDeviceThatPlaysDolbyVisionKeepsIt() {
        assertFalse(DolbyVisionBaseLayer.playAsHevc(dv, "dvhe.07.06", true, deviceHandlesTrack = true))
    }

    @Test
    fun otherProfilesAreLeftToMedia3() {
        for (codecs in listOf("dvhe.05.06", "dvhe.08.06", "dvav.09.05", "dav1.10.09", "dvhe.17.06", "xdvhe.07.06", null)) {
            assertFalse("$codecs", DolbyVisionBaseLayer.playAsHevc(dv, codecs, true, deviceHandlesTrack = false))
        }
    }

    @Test
    fun aFileThatSaysItIsNotHdr10IsNotTouched() {
        assertFalse(DolbyVisionBaseLayer.playAsHevc(dv, "dvhe.07.06", false, deviceHandlesTrack = false))
    }

    // an HEVC NAL unit in Annex B: start code, two header bytes (type in bits 1..6 of the first), payload
    private fun unit(type: Int, vararg payload: Int, short: Boolean = false): List<Int> =
        (if (short) listOf(0, 0, 1) else listOf(0, 0, 0, 1)) + listOf(type shl 1, 1) + payload.toList()

    private fun strip(vararg units: List<Int>, lead: List<Int> = emptyList(), tail: List<Int> = emptyList()): List<Int> {
        val bytes = (lead + units.flatMap { it } + tail).map { it.toByte() }.toByteArray()
        // inside a larger array, at an offset, as a codec buffer's content can be
        val room = ByteArray(bytes.size + 7) { 0x55 }
        System.arraycopy(bytes, 0, room, 3, bytes.size)
        val kept = DolbyVisionBaseLayer.stripLayers(room, 3, bytes.size)
        assertEquals("bytes after the unit are not touched", 0x55.toByte(), room[3 + bytes.size])
        return room.copyOfRange(3, 3 + kept).map { it.toInt() and 0xFF }
    }

    @Test
    fun theRpuAndTheEnhancementLayerAreRemovedAndTheBaseLayerKept() {
        // as a profile 7 access unit: AUD 35, a slice 1 of the base layer, the RPU 62, an EL 63 wrapping a slice
        val aud = unit(35, 0x50)
        val slice = unit(1, 0xAF, 0x00, 0x00, 0x03, 0x01, 0x22)
        val rpu = unit(62, 0x01, 0x10, 0x00, 0x00, 0x03)
        val el = unit(63, 0x02, 0x01, 0xD0, 0x99, short = false)
        assertEquals(aud + slice, strip(aud, rpu, slice, el))
    }

    @Test
    fun aUnitWithoutDolbyVisionStaysAsItWas() {
        val vps = unit(32, 0x0C, 0x01)
        val sps = unit(33, 0x01, 0x01, short = true)
        val slice = unit(19, 0xAF, 0x88)
        assertEquals(vps + sps + slice, strip(vps, sps, slice))
    }

    @Test
    fun theLastUnitAndBytesBeforeTheFirstStartCodeAreHandled() {
        val slice = unit(1, 0xAF)
        assertEquals(listOf(9, 9) + slice, strip(slice, unit(62, 0x01, 0x02), lead = listOf(9, 9)))
        assertEquals(emptyList<Int>(), strip(unit(63, 0x01), unit(62)))
    }

    @Test
    fun plainHevcIsNotTouched() {
        assertFalse(DolbyVisionBaseLayer.playAsHevc("video/hevc", "dvhe.07.06", true, deviceHandlesTrack = false))
        assertFalse(DolbyVisionBaseLayer.playAsHevc(null, "dvhe.07.06", true, deviceHandlesTrack = false))
    }
}
