# The binary is built outside (static, CGO_ENABLED=0) by deploy/zimaos/build-on-host.sh,
# so the image holds nothing but it: no shell, no package manager, no base image to age.
FROM scratch
# --chmod: the mode must not depend on the umask of whoever built the binary
# (0700 root under cap_drop ALL = "permission denied", measured 2026-10-28).
COPY --chmod=0755 movies-cast /movies-cast
USER 65534:65534
EXPOSE 8097
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/movies-cast", "-healthcheck"]
ENTRYPOINT ["/movies-cast"]
