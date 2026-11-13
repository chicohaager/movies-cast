package io.github.chicohaager.moviescast

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

    @Test
    fun plainHevcIsNotTouched() {
        assertFalse(DolbyVisionBaseLayer.playAsHevc("video/hevc", "dvhe.07.06", true, deviceHandlesTrack = false))
        assertFalse(DolbyVisionBaseLayer.playAsHevc(null, "dvhe.07.06", true, deviceHandlesTrack = false))
    }
}
