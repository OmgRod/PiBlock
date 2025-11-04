use anyhow::Result;
use std::sync::Arc;
use tokio::net::UdpSocket;
use trust_dns_proto::op::{Message, ResponseCode};
use trust_dns_proto::rr::{RData, Record, RecordType, DNSClass};
use trust_dns_proto::rr::rdata::A as ARecord;
use std::time::Duration;
use crate::state::ServerState;
use crate::blocklist::is_blocked_domain;
use std::net::Ipv4Addr;
use std::sync::atomic::Ordering;

pub async fn run_udp_server(state: Arc<ServerState>, bind_addr: String, udp_upstream: String) -> Result<()> {
    let sock = UdpSocket::bind(bind_addr.as_str()).await?;
    let sock = Arc::new(sock);
    tracing::info!("DNS UDP listening on {}", bind_addr);
    loop {
        let mut buf = vec![0u8; 4096];
        let (len, src) = sock.recv_from(&mut buf).await?;
        let packet = buf[..len].to_vec();
    let state_cl = state.clone();
    let upstream = udp_upstream.clone();
        let sock_cl = sock.clone();
        tokio::spawn(async move {
            state_cl.queries.fetch_add(1, Ordering::Relaxed);
            match Message::from_vec(&packet) {
                Ok(msg) => {
                    if let Some(q) = msg.queries().first() {
                        let qname = q.name().to_string();
                        let lists = state_cl.lists.read().await.clone();
                        if is_blocked_domain(&qname, &lists) {
                            state_cl.blocked.fetch_add(1, Ordering::Relaxed);
                            let mode = state_cl.mode.read().await.clone();
                            let block_ip_opt = state_cl.block_page_ip.read().await.clone();
                            match mode.as_str() {
                                "redirect" => {
                                    if let Some(ipstr) = block_ip_opt {
                                        if let Ok(ipv4) = ipstr.parse::<Ipv4Addr>() {
                                            let mut resp = Message::new();
                                            resp.set_id(msg.id());
                                            resp.set_message_type(trust_dns_proto::op::MessageType::Response);
                                            resp.set_op_code(msg.op_code());
                                            resp.set_response_code(ResponseCode::NoError);
                                            if let Some(q) = msg.queries().first() {
                                                let mut rec = Record::new();
                                                rec.set_name(q.name().clone());
                                                rec.set_rr_type(RecordType::A);
                                                rec.set_dns_class(DNSClass::IN);
                                                rec.set_ttl(60);
                                                rec.set_data(Some(RData::A(ARecord(ipv4))));
                                                resp.add_answer(rec);
                                            }
                                            if let Ok(out) = resp.to_vec() {
                                                let _ = sock_cl.send_to(&out, &src).await;
                                            }
                                            return;
                                        }
                                    }
                                    let resp = Message::error_msg(msg.id(), msg.op_code(), ResponseCode::NXDomain);
                                    if let Ok(out) = resp.to_vec() { let _ = sock_cl.send_to(&out, &src).await; }
                                    return;
                                }
                                "null" => {
                                    let mut resp = Message::new();
                                    resp.set_id(msg.id());
                                    resp.set_message_type(trust_dns_proto::op::MessageType::Response);
                                    resp.set_op_code(msg.op_code());
                                    resp.set_response_code(ResponseCode::NoError);
                                    if let Some(q) = msg.queries().first() {
                                        let mut rec = Record::new();
                                        rec.set_name(q.name().clone());
                                        rec.set_rr_type(RecordType::A);
                                        rec.set_dns_class(DNSClass::IN);
                                        rec.set_ttl(60);
                                        rec.set_data(Some(RData::A(ARecord(Ipv4Addr::new(0,0,0,0)))));
                                        resp.add_answer(rec);
                                    }
                                    if let Ok(out) = resp.to_vec() { let _ = sock_cl.send_to(&out, &src).await; }
                                    return;
                                }
                                _ => {
                                    let resp = Message::error_msg(msg.id(), msg.op_code(), ResponseCode::NXDomain);
                                    if let Ok(out) = resp.to_vec() { let _ = sock_cl.send_to(&out, &src).await; }
                                    return;
                                }
                            }
                        }
                    }
                    if let Ok(up_resp) = forward_udp_to_upstream(&packet, &upstream).await {
                        let _ = sock_cl.send_to(&up_resp, &src).await;
                    }
                }
                Err(_) => {
                    // ignore unparsable packets
                }
            }
        });
    }
}

pub async fn forward_udp_to_upstream(pkt: &[u8], upstream: &str) -> Result<Vec<u8>> {
    let up = UdpSocket::bind(("0.0.0.0", 0)).await?;
    up.send_to(pkt, upstream).await?;
    let mut buf = vec![0u8; 4096];
    let res = tokio::time::timeout(Duration::from_secs(3), up.recv_from(&mut buf)).await;
    match res {
        Ok(Ok((n, _))) => Ok(buf[..n].to_vec()),
        _ => Err(anyhow::anyhow!("upstream timeout")),
    }
}
