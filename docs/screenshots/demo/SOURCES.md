# Demo library for the README screenshots

The dev server shows these films instead of the captured test library when started with
`MC_DEV_LIBRARY=../../docs/screenshots/demo` (see the README). The Go tests and the browser tests do not use it.

- **Title, year, running time, one-line description:** Wikidata (CC0), item named per film below, fetched 2026-10-29.
- **Pictures:** Wikimedia Commons, each file marked "Public domain" there (license as Commons states it, fetched 2026-10-29),
  scaled to 400 px wide JPEG. Nosferatu has no public-domain poster on Commons; its picture is a frame of the film.
- **File details** (container, codecs, size, subtitles) are not the films' — they are copied from one captured
  Movies answer so that the format check and the player work; the "HEVC · AC3" in the dialog is that demo value.

| Film | Wikidata | Picture | Commons page | License (Commons) | Author (Commons) |
|---|---|---|---|---|---|
| Nosferatu (1922) | [Q151895](https://www.wikidata.org/wiki/Q151895) | `posters/nosferatu.jpg` | [Max Schreck as Count Orlok in Nosferatu – Eine Symphonie des Grauens (1922).jpg](https://commons.wikimedia.org/wiki/File:Max_Schreck_as_Count_Orlok_in_Nosferatu_%E2%80%93_Eine_Symphonie_des_Grauens_(1922).jpg) | Public domain | F. W. Murnau |
| The Cabinet of Dr. Caligari (1920) | [Q153886](https://www.wikidata.org/wiki/Q153886) | `posters/caligari.jpg` | [CABINETOFDRCALIGARI-poster.jpg](https://commons.wikimedia.org/wiki/File:CABINETOFDRCALIGARI-poster.jpg) | Public domain | Unknown author |
| The General (1926) | [Q850159](https://www.wikidata.org/wiki/Q850159) | `posters/general.jpg` | [The General (1926) - Movie Poster 2.png](https://commons.wikimedia.org/wiki/File:The_General_(1926)_-_Movie_Poster_2.png) | Public domain | United Artists Corporation |
| A Trip to the Moon (1902) | [Q244971](https://www.wikidata.org/wiki/Q244971) | `posters/moon.jpg` | [Voyage dans la Lune affiche.jpg](https://commons.wikimedia.org/wiki/File:Voyage_dans_la_Lune_affiche.jpg) | Public domain | Georges Méliès |
| Sherlock, Jr. (1924) | [Q1569918](https://www.wikidata.org/wiki/Q1569918) | `posters/sherlock.jpg` | [Sherlock jr poster.jpg](https://commons.wikimedia.org/wiki/File:Sherlock_jr_poster.jpg) | Public domain | Unknown author |
| Night of the Living Dead (1968) | [Q623051](https://www.wikidata.org/wiki/Q623051) | `posters/notld.jpg` | [Night Of The Living Dead (1968) - Poster.jpg](https://commons.wikimedia.org/wiki/File:Night_Of_The_Living_Dead_(1968)_-_Poster.jpg) | Public domain | Unknown author |
