CREATE TABLE public.ticker_ids
(
    ticker character varying(16) NOT NULL,
    id character varying(128) NOT NULL
)
WITH (
    OIDS = FALSE
);